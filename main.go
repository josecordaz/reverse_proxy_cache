package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/siddontang/go/log"

	"github.com/pinpt/go-common/hash"
	pos "github.com/pinpt/go-common/os"
)

func setHeader(name string, r *http.Request) {
	if obj := r.Header[name]; len(obj) > 0 {
		if nameValue := obj[0]; nameValue != "" {
			r.Header.Add(name, nameValue)
		}
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {

	// pos.

	redisClient := redis.NewClient(&redis.Options{
		Addr:            "localhost:6379",
		Password:        "", // no password set
		DB:              0,  // use default DB
		DialTimeout:     10 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		PoolSize:        10,
		PoolTimeout:     30 * time.Second,
		MaxRetries:      5,
		MinRetryBackoff: time.Second * 3,
		MaxRetryBackoff: time.Second * 6,
	})

	defer redisClient.Close()

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

	redisValue, err := redisClient.Get(requestHash).Result()
	if err != nil && err != redis.Nil {
		log.Error("Err", err)
		panic(err)
	}

	var responseBytes []byte

	if redisValue == "" {

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

		err = redisClient.Set(requestHash, string(responseBytes), 0).Err()
		if err != nil {
			log.Error("Err", err)
			panic(err)
		}

		btsHeader, err := json.Marshal(response.Header)
		if err != nil {
			log.Error("Err", btsHeader)
		}

		log.Info(fmt.Sprintf("setting headers %s => %s ", requestHash, string(btsHeader)))

		err = redisClient.Set(requestHash+"headers", string(btsHeader), 0).Err()
		if err != nil {
			log.Error("Err", err)
			panic(err)
		}

		log.Info(fmt.Sprintf("saved key in cache [%s]", requestHash))

		log.Debug(fmt.Sprintf("response => %s", string(responseBytes)))

		for keyHeader, valueHeader := range response.Header {
			w.Header().Set(keyHeader, strings.Join(valueHeader, ","))
		}
	} else {
		log.Debug(fmt.Sprintf("using cache [%s] = > %s", requestHash, redisValue))
		responseBytes = []byte(redisValue)

		headersValue, err := redisClient.Get(requestHash + "headers").Result()
		if err != nil && err != redis.Nil {
			log.Error("Err", err)
			panic(err)
		}

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

type cache struct {
	sync.Mutex
	mapa   map[string]string
	cached bool
}

func (c *cache) Set(key string, value string) {
	c.Lock()
	c.mapa[key] = value
	defer c.Unlock()
}

func (c *cache) Get(key string) string {
	c.Lock()
	defer c.Unlock()
	return c.mapa[key]
}

func main() {

	var cache cache

	cache.Set("esto", "eso")

	http.HandleFunc("/", mainHandler)

	pos.OnExit(func(_ int) {
		log.Info("Saving cache locally")
		time.Sleep(time.Second * 20)
		// cancel()
	})

	if len(os.Args) > 1 {
		log.SetLevelByName(os.Args[1])
	} else {
		log.SetLevelByName("info")
	}

	fmt.Println("server running")
	err := http.ListenAndServe(":3645", nil)

	if err != nil {
		log.Error("Err", err)
	}
}
