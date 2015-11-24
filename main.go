// Package main implements the main functionality of Gofetch.
package main

import (
	"github.com/op/go-logging"
	"sync"
	"time"
)

// testGofetch must be true when testing to use the appropriate S3 folders.
var testGofetch = false

var log = logging.MustGetLogger("gofetch")

func main() {
	mainStart := time.Now()
	ConfigureLogger()
	CheckEnvVars()
	log.Info("Starting gofetch.")
	config := ConfigFromS3()

	if len(config.Urls) == 0 {
		panic("No URLs found in the configuration file.")
	}

	throttleMap := ThrottleMap(config.Throttlers)
	throttled := len(throttleMap)
	concWriters := ConcurrentS3Writes()
	concFetches := ConcurrentFetches(throttled)
	fetchOffset := FetchOffset()
	fetchLimit := FetchLimit()

	// Checking configuration file URLs to avoid over allocating memory.
	actualLimit := fetchOffset + fetchLimit
	if actualLimit > len(config.Urls) {
		log.Notice("Forcing fetching limit to %d (instead of %d).", len(config.Urls), actualLimit)
		actualLimit = len(config.Urls)
	}

	fetchRange := actualLimit - fetchOffset
	log.Notice("Fetching %d URLs of %d in configuration file.", actualLimit-fetchOffset, len(config.Urls))

	// s3chan stores up to 100 buffered HttpResponses.
	s3chan := make(chan *HTTPFetch, 100)
	// fetchChan stores the up to X concurrent scrapes, allows to block when we've reached capacity.
	fetchChan := make(chan *URLInfo, concFetches)
	// logChan stores all the fetch logs as a result of the overall fetch.
	logChan := make(chan *Fetch, fetchRange)
	// errChan stores all the fetch errors. It is as long as the logChan in case all fetches fail.
	errChan := make(chan *FetchError, fetchRange)

	// Using a wait group to make sure not to die prior to all URLs fetched.
	var wg sync.WaitGroup

	ConfigureRuntime()
	// Starting as many concurrent scrapers as requested.
	for i := 0; i < concFetches; i++ {
		go Fetcher(fetchChan, s3chan, errChan, throttleMap, &wg)
	}

	// Putting all URLs to fetch to the fetch channel, as determined by the environment.
	for _, urlI := range config.Urls[fetchOffset:fetchRange] {
		wg.Add(1)
		go func(urlI *URLInfo) {
			fetchChan <- urlI
		}(urlI)
	}
	// The fetchChan is closed after everything has been processed because failure to process
	// a fetch will add it to the channel again.

	// Starting the S3 processor.
	for i := 0; i < concWriters; i++ {
		go ProcessResponses(s3chan, logChan, config.Indexes, &wg)
	}

	// Wait for completion of both fetching and writing content to S3.
	wg.Wait()

	close(fetchChan)
	close(s3chan)
	close(logChan)
	close(errChan)

	fetchDuration := time.Now().Sub(mainStart)
	// Write the log completion file to S3.
	LogFetches(logChan, errChan, &fetchDuration)
	log.Info("Successfully completed gofetch in %s.", fetchDuration)
}
