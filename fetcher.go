package main

import (
	"crypto/sha512"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPFetch stores a response from http.Get.
type HTTPFetch struct {
	urlInfo   *URLInfo       // Stores the UrlInfo which initiated the request.
	response  *http.Response // Stores the response object.
	body      []byte         // Stores the body so we can close the IO.
	startTime time.Time      // Stores the start time of the fetch.
	duration  time.Duration  // Stores the duration of the fetch in nanoseconds.
	checksum  string         // Stores the sha384 checksum of the body.
}

// HTTPThrottler stores throttling information with the delay between requests and the latest fetch.
type HTTPThrottler struct {
	delay       time.Duration // Stores the delay between requests to a given host.
	latestFetch time.Time     // Stores the time of the latest fetch.
}

// Fetcher fetches a given URL. The result is put on the provided channel.
func Fetcher(fetchChan <-chan *URLInfo, s3chan chan<- *HTTPFetch, errChan chan<- *FetchError, throttleMap map[string]*HTTPThrottler, wg *sync.WaitGroup) {
	for {
		urlInfo, more := <-fetchChan
		if !more {
			log.Info("No more URLs to process.")
			return
		}
		start := time.Now()
		cleanURL := strings.Replace(strings.TrimSpace(urlInfo.Link), " ", "+", -1)

		// Check if this host needs throttling.
		parsedURL, _ := url.Parse(cleanURL) // Note that we do not catch any error here since it will be caught on the GET
		if throttle := throttleMap[parsedURL.Host]; throttle != nil {
			time.Sleep(throttle.delay - time.Now().Sub(throttle.latestFetch))
			throttle.latestFetch = time.Now() // Updating the latestFetch is sufficient since the map is for a ref.
		}

		// Fetch the URL and catch any error.
		resp, err := http.Get(cleanURL)
		if err != nil {
			errChan <- &FetchError{Cleaned: cleanURL, Original: urlInfo.Link, Message: err.Error()}
			log.Critical("Error fetching %s: %s.", cleanURL, err)
			wg.Done() // Decrement the counter so as to not wait for this item to be processed.
			continue
		}

		// Computing the duration of the request now.
		duration := time.Now().Sub(start)
		// Read the response body, and close it.
		respBody, ioerr := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if ioerr != nil {
			panic(ioerr)
		}
		// Computing the SHA384 checksum.
		hash := sha512.New384()
		hash.Write(respBody)
		checksum := hex.EncodeToString(hash.Sum(nil))
		s3chan <- &HTTPFetch{urlInfo: urlInfo, response: resp, body: respBody, startTime: start, duration: duration, checksum: checksum}
	}
}
