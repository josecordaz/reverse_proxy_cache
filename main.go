package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/siddontang/go/log"

	"github.com/pinpt/go-common/hash"
	iop "github.com/pinpt/go-common/io"
	pos "github.com/pinpt/go-common/os"
)

type storage interface {
	Get(key string) string
	Set(key string, value string)
}

func myHanlder(w http.ResponseWriter, r *http.Request, cache storage) {

	client := &http.Client{}

	newURL := r.Header["X-Host"][0]

	log.Info(fmt.Sprintf("url %s", newURL))

	postBodyBts, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Err", err)
	}
	r.Body.Close()
	bodyReader := bytes.NewReader(postBodyBts)
	r.Body = ioutil.NopCloser(bodyReader)

	requestHash := hash.Values(newURL, r.Method, string(postBodyBts))
	log.Info(fmt.Sprintf("hash[%s]", requestHash))

	cacheBody := cache.Get(requestHash)

	var responseBytes []byte

	if cacheBody == "" {

		newReq, err := http.NewRequest(r.Method, newURL, r.Body)
		if err != nil {
			log.Error("Err", err)
		}

		r.Header.Del("X-Host")

		newReq.Header = r.Header

		response, err := client.Do(newReq)

		log.Info("status", response.Status)

		defer func() {
			err := response.Body.Close()
			if err != nil {
				log.Error("Err", err)
			}
		}()

		responseBytes, err = ioutil.ReadAll(response.Body)
		if err != nil {
			log.Errorf("Err", err)
		}

		cache.Set(requestHash, string(responseBytes))

		btsHeader, err := json.Marshal(response.Header)
		if err != nil {
			log.Error("Err", btsHeader)
		}

		log.Info(fmt.Sprintf("setting headers %s => %s ", requestHash, string(btsHeader)))

		cache.Set(requestHash+"headers", string(btsHeader))

		log.Info(fmt.Sprintf("saved key in cache [%s]", requestHash))

		log.Debug(fmt.Sprintf("response => %s", string(responseBytes)))

		for keyHeader, valueHeader := range response.Header {
			w.Header().Set(keyHeader, strings.Join(valueHeader, ","))
		}
	} else {
		log.Debug(fmt.Sprintf("using cache [%s] = > %s", requestHash, cacheBody))
		responseBytes = []byte(cacheBody)

		headersValue := cache.Get(requestHash + "headers")

		var headers map[string][]string

		err = json.Unmarshal([]byte(headersValue), &headers)
		if err != nil {
			log.Error("Err", err)
		}

		for keyHeader, valueHeader := range headers {
			w.Header().Set(keyHeader, strings.Join(valueHeader, ","))
		}
	}

	w.Write(responseBytes)
}

func mainHandler(w http.ResponseWriter, r *http.Request) {

	cache := &cache{
		mapa: make(map[string]*string),
	}

	myHanlder(w, r, cache)

}

type cache struct {
	sync.Mutex
	mapa map[string]*string
}

func (c *cache) Set(key string, value string) {
	c.Lock()
	c.mapa[key] = &value
	defer c.Unlock()
}

func (c *cache) Get(key string) string {
	c.Lock()
	defer c.Unlock()
	return *c.mapa[key]
}

func loadCache(cache *cache) {

	cacheFile, err := os.Open(cacheFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer cacheFile.Close()

	scanner := bufio.NewScanner(cacheFile)
	for scanner.Scan() {
		var cacheRecord CacheRecord
		if err := json.Unmarshal(scanner.Bytes(), &cacheRecord); err != nil {
			log.Error("Err", err)
		}
		cache.mapa[cacheRecord.key] = &cacheRecord.value
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

}

const cacheFileName = "storage.cache"

type CacheRecord struct {
	key   string
	value string
}

func main() {

	var cache cache

	cache.mapa = make(map[string]*string)

	log.Info("Loading cache")

	loadCache(&cache)

	// TODO generate this code
	// Read all the info in the file and send it to the map
	log.Info("Done")

	// ticker := time.NewTicker(30 * time.Second)
	// quit := make(chan struct{})
	// go func() {
	// 	for {
	// 		select {
	// 		case <-ticker.C:
	// 			log.Info("Saving cache to file")
	// 			only10 := 0
	// 			for _, cacheElement := range cache.mapa {
	// 				if !cacheElement.cached {
	// 					stream.Write(cacheElement)
	// 					cacheElement.cached = true
	// 					only10++
	// 					if only10 == 10 {
	// 						break
	// 					}
	// 				}
	// 			}
	// 		case <-quit:
	// 			ticker.Stop()
	// 			return
	// 		}
	// 	}
	// }()

	http.HandleFunc("/", mainHandler)

	pos.OnExit(func(_ int) {
		// close(quit)
		log.Info("Saving cache to file")
		stream, err := iop.NewJSONStream(cacheFileName)
		if err != nil {
			panic(err)
		}
		defer stream.Close()

		for cacheKey, cacheValue := range cache.mapa {
			stream.Write(CacheRecord{cacheKey, *cacheValue})
		}
		log.Info("Done")
	})

	if len(os.Args) > 1 {
		log.SetLevelByName(os.Args[1])
	} else {
		log.SetLevelByName("info")
	}

	log.Info("server running")
	err := http.ListenAndServe(":3645", nil)
	if err != nil {
		log.Error("Err", err)
	}
}
