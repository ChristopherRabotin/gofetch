package main

import (
	"errors"
	"fmt"
	"github.com/op/go-logging"
	"os"
	"runtime"
	"strconv"
	"time"
)

// CheckEnvVars checks that all the environment variables required are set, without checking their value. It will panic if one is missing.
func CheckEnvVars() {
	envvars := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_STORAGE_BUCKET_NAME", "AWS_CONFIG_FILE", "FETCH_ID", "FETCH_OFFSET", "FETCH_LIMIT"}
	for _, envvar := range envvars {
		if os.Getenv(envvar) == "" {
			panic(fmt.Errorf("environment variable `%s` is missing or empty,", envvar))
		}
	}
}

// ConfigureRuntime configures the server runtime, including the number of CPUs to use.
func ConfigureRuntime() {
	// Note that we're using os instead of syscall because we'll be parsing the int anyway, so there is no need to check if the envvar was found.
	useNumCPUs := intFromEnvVar("MAX_CPUS", int(runtime.NumCPU()))
	runtime.GOMAXPROCS(useNumCPUs)
	log.Notice("Running with %d CPUs.\n", useNumCPUs)
}

// ConfigureLogger configures the default logger (named "gofetch").
func ConfigureLogger() {
	// From https://github.com/op/go-logging/blob/master/examples/example.go.
	logFormat := logging.MustStringFormatter("%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level}%{color:reset} %{message}")
	logging.SetBackend(logging.NewBackendFormatter(logging.NewLogBackend(os.Stderr, "", 0), logFormat))
	// Let's grab the log level from the environment, or set it to INFO.
	envlvl := os.Getenv("LOG_LEVEL")
	if envlvl != "" {
		lvl, err := logging.LogLevel(envlvl)
		if err == nil {
			log.Notice("Set logging level to %s.\n", lvl)
			logging.SetLevel(lvl, "")
		} else {
			fmt.Errorf("%s", err)
		}
	} else {
		log.Notice("No log level defined in environment. Defaulting to INFO.\n")
		logging.SetLevel(logging.INFO, "")
	}
}

// ConcurrentFetches returns the number of fetching go routines to start.
func ConcurrentFetches(throttled int) int {
	concurrency := intFromEnvVar("CONCURRENT_FETCHES", 25) + throttled
	log.Notice("Running with %d fetching go routines (including %d for throttled hosts).\n", concurrency, throttled)
	return concurrency
}

// ConcurrentS3Writes returns the number of S3 writing go routines to start.
func ConcurrentS3Writes() int {
	concurrency := intFromEnvVar("CONCURRENT_S3WRITERS", 4)
	log.Notice("Running with %d S3 writing go routines.\n", concurrency)
	return concurrency
}

// FetchOffset returns the fetch offset as defined in the environment. May panic.
func FetchOffset() int {
	offset := intFromEnvVar("FETCH_OFFSET", -1)
	if offset < 0 {
		panic(errors.New("FETCH_OFFSET could not be parsed or is a negative number"))
	}
	return offset
}

// FetchLimit returns the fetch limit as defined in the environment. May panic.
func FetchLimit() int {
	limit := intFromEnvVar("FETCH_LIMIT", -1)
	if limit < 0 {
		panic(errors.New("FETCH_LIMIT could not be parsed or is a negative number"))
	}
	return limit
}

// ThrottleMap returns a map of string to HTTPThrottler allows for O(1) host lookup.
func ThrottleMap(throttlers []*Throttler) map[string]*HTTPThrottler {
	throttledMap := make(map[string]*HTTPThrottler)
	for _, throttle := range throttlers {
		delay, err := throttle.GetDuration()
		if err != nil {
			continue // Error is logged in Duration function.
		}
		// Initialize the latest fetch to yesterday.
		throttledMap[throttle.Host] = &HTTPThrottler{delay: delay, latestFetch: time.Now().AddDate(0, 0, -1)}
	}
	return throttledMap
}

// intFromEnvVar return the requested environment variable as an integer, or the default value.
func intFromEnvVar(envvar string, deflt int) int {
	// Note that we're using os instead of syscall because we'll be parsing the int anyway, so there is no need to check if the envvar was found.
	envVarStr := os.Getenv(envvar)
	envVarInt, err := strconv.ParseInt(envVarStr, 10, 0)
	if err != nil {
		return deflt
	}
	return int(envVarInt)
}
