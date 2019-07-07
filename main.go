package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/go-redis/redis"
	"github.com/siddontang/go/log"

	"github.com/pinpt/go-common/hash"
)

var redisClient *redis.Client

func init() {

	// Output: PONG <nil>
}

func setHeader(name string, r *http.Request) {
	if obj := r.Header[name]; len(obj) > 0 {
		if nameValue := obj[0]; nameValue != "" {
			r.Header.Add(name, nameValue)
		}
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {

	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	defer redisClient.Close()

	// _, err := redisClient.Ping().Result()
	// if err != nil {
	// 	log.Error("Err", err)
	// 	os.Exit(1)
	// }

	client := &http.Client{}

	for k, v := range r.Header {
		log.Info("Header "+k, v)
	}

	newURL := r.Header["X-Host"][0]

	log.Info(fmt.Sprintf("url %s", newURL))

	requestHash := hash.Values(newURL, r.Method)
	log.Info(fmt.Sprintf("hash[%s]", requestHash))

	redisValue, err := redisClient.Get(requestHash).Result()
	if err != nil && err != redis.Nil {
		panic(err)
	}

	var responseBytes []byte

	if redisValue == "" {

		newReq, err := http.NewRequest(r.Method, newURL, r.Body)
		if err != nil {
			log.Error("Err", err)
		}

		setHeader("Authorization", r)
		setHeader("Content-Type", r)

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
		}

		btsHeader, err := json.Marshal(response.Header)
		if err != nil {
			log.Error("Err", btsHeader)
		}

		log.Info(fmt.Sprintf("setting headers %s => %s ", requestHash, string(btsHeader)))

		err = redisClient.Set(requestHash+"headers", string(btsHeader), 0).Err()
		if err != nil {
			log.Error("Err", err)
		}

		log.Info(fmt.Sprintf("saved key in cache [%s]", requestHash))

		log.Info(fmt.Sprintf("response => %s", string(responseBytes)))

		for keyHeader, valueHeader := range response.Header {
			w.Header().Set(keyHeader, strings.Join(valueHeader, ","))
		}
	} else {
		log.Info(fmt.Sprintf("using cache [%s] = > %s", requestHash, redisValue))
		responseBytes = []byte(redisValue)

		headersValue, err := redisClient.Get(requestHash + "headers").Result()
		if err != nil && err != redis.Nil {
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

func main() {
	http.HandleFunc("/", mainHandler)

	fmt.Println("server running")
	err := http.ListenAndServe(":3645", nil)

	if err != nil {
		log.Error("Err", err)
	}
}
