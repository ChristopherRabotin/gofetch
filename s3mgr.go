package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
)

// Config allows for unmarshling of the remote configuration file.
type Config struct {
	XMLName    xml.Name     `xml:"config"`
	Indexes    []*Index     `xml:"index"`
	Throttlers []*Throttler `xml:"throttle"`
	Urls       []*URLInfo   `xml:"urls>url"`
}

// Index stores the index information, with their name and enable status.
type Index struct {
	XMLName xml.Name `xml:"index"`
	Enabled bool     `xml:"enabled,attr"`
	Name    string   `xml:"name,attr"`
}

// URLInfo stores the URL info which is to be fetched.
type URLInfo struct {
	XMLName xml.Name `xml:"url"`
	Link    string   `xml:"link"`
	Parser  Parser   `xml:",any"`
}

// Parser stores the parse meta data, which will be written back in the output log.
type Parser struct {
	XMLName xml.Name
	Name    string `xml:"name,attr"`
	XML     string `xml:",innerxml"`
}

// Throttler stores the throttle information read from the configuration file.
type Throttler struct {
	Host  string  `xml:"host,attr"`
	Unit  string  `xml:"unit,attr"`
	Delay float64 `xml:"delay,attr"`
}

// GetDuration validates and returns the parsed duration.
func (throttle Throttler) GetDuration() (delay time.Duration, err error) {
	delayStr := fmt.Sprintf("%f%s", throttle.Delay, throttle.Unit)
	delay, err = time.ParseDuration(delayStr)
	if err != nil {
		log.Critical("Could not parse duration %s: %s", delayStr, err.Error())
	}
	return
}

// Fetches allows for marshling of output log.
type Fetches struct {
	XMLName    xml.Name      `xml:"fetches"`
	Fetch      []*Fetch      `xml:"fetch"`
	FetchError []*FetchError `xml:"error"`
	Meta       *Meta         `xml:"meta"`
}

// Fetch allows for marshling of single fetch result in output log.
type Fetch struct {
	Novel         bool       `xml:"novel,attr"`
	Parser        string     `xml:"parser,attr"`
	ChecksumIndex S3Location `xml:"checksumIndex"`
	S3Content     S3Location `xml:"s3content"`
	ParserData    Parser     `xml:"parser"`
}

// FetchError allows for marshling of a fetching error.
type FetchError struct {
	Original string `xml:"original_link,attr"`
	Cleaned  string `xml:"clean_link,attr"`
	Message  string `xml:"message,attr"`
}

// Meta allows for marshling of the meta information of a run.
type Meta struct {
	Report        *Report        `xml:"report"`
	FetchDuration *FetchDuration `xml:"duration"`
}

// FetchDuration allows for marshling of the duration of a run.
type FetchDuration struct {
	Hours   float64 `xml:"hours,attr"`
	Minutes float64 `xml:"minutes,attr"`
	Seconds float64 `xml:"seconds,attr"`
}

// Report allows for marshling of the report of a run.
type Report struct {
	Novel  int `xml:"novel,attr"`
	Errors int `xml:"errors,attr"`
	Total  int `xml:"total,attr"`
}

// S3Location allows for marshling of a file location on S3.
type S3Location struct {
	Bucket string `xml:"bucket,attr"`
	Path   string `xml:"path,attr"`
}

// S3BucketFromOS returns the bucket from the environment variables (cf. README.md).
func S3BucketFromOS() *s3.Bucket {
	// Prepare AWS S3 connection.
	s3auth, err := aws.EnvAuth()
	if err != nil {
		log.Fatal(err)
	}
	client := s3.New(s3auth, aws.USEast)
	return client.Bucket(os.Getenv("AWS_STORAGE_BUCKET_NAME"))
}

// ConfigFromS3 reads the config file from AWS S3, from the environment variables.
//
// From a given bucket and a configPath, ConfigFromS3 will return a Config
// struct which is an exact representation of the XML file.
// WARNING: May panic if config is not found or data cannot be unmarshalled.
func ConfigFromS3() *Config {
	bucket := S3BucketFromOS()
	configPath := os.Getenv("AWS_CONFIG_FILE")
	configBody, notFoundErr := bucket.Get(configPath)
	if notFoundErr != nil {
		// Oops, couldn't find the configuration file! Gotta panic now!
		panic(notFoundErr)
	}
	config := Config{}
	xmlErr := xml.Unmarshal(configBody, &config)
	if xmlErr != nil {
		// Oops, couldn't read the configuration file! Gotta panic now!
		panic(xmlErr)
	}
	return &config
}

// ProcessResponses processes all the HTTPFetch and writes the content and indexes to S3.
func ProcessResponses(s3chan chan *HTTPFetch, logChan chan<- *Fetch, indexes []*Index, wg *sync.WaitGroup) {
	bucket := S3BucketFromOS()
	for {
		fetch, open := <-s3chan
		if !open {
			log.Info("Done processing responses: the s3chan is closed.")
			return
		}
		log.Debug("%s was fetched (status=%s) in %s.\n", fetch.urlInfo.Link, fetch.response.Status, fetch.duration)
		rootPath := "/gofetch"
		if testGofetch {
			rootPath += "/test_data"
		}
		contentPath := fmt.Sprintf("%s/sha384_content/%s", rootPath, fetch.checksum)
		// Check whether the checksum is in the canonical index.
		idx := CanonicalIndex{}
		indexData, notFoundErr := bucket.Get(idx.Path(fetch, rootPath))
		if notFoundErr == nil {
			// Append index content to the index.
			indexData := string(indexData) + idx.Content(fetch, contentPath)
			s3Err := bucket.Put(idx.Path(fetch, rootPath), []byte(indexData), "text/plain", s3.Private)
			if s3Err != nil {
				// If somethting goes wrong, let's re-add this fetch to items to be processed.
				s3chan <- fetch
				log.Error("Could not update index: %s", s3Err)
				continue
			}
			// Log the success.
			logChan <- &Fetch{Novel: false, Parser: fetch.urlInfo.Parser.Name, ChecksumIndex: S3Location{Bucket: bucket.Name, Path: idx.Path(fetch, rootPath)}, S3Content: S3Location{Bucket: bucket.Name, Path: contentPath}, ParserData: fetch.urlInfo.Parser}

		} else {
			// Store the content on S3. Note that we set *all* content types to text/plain.
			s3Err := bucket.Put(contentPath, fetch.body, "text/plain", s3.Private)
			if s3Err != nil {
				// If somethting goes wrong, let's re-add this fetch to items to be processed.
				s3chan <- fetch
				log.Error("Could not PUT new content: %s", s3Err)
				continue
			}

			// Add canonical index information.
			for i := 0; i < 10; i++ {
				s3Err = bucket.Put(idx.Path(fetch, rootPath), []byte(idx.Content(fetch, contentPath)), "text/plain", s3.Private)
				if s3Err == nil {
					break
				} else if i == 9 {
					// Panic: we have attempted to add the index information ten times.
					panic(fmt.Sprintf("Could not add index: path=%s ; content=[%s]", idx.Path(fetch, rootPath), idx.Content(fetch, contentPath)))
				}
			}

			// Log the success.
			logChan <- &Fetch{Novel: true, Parser: fetch.urlInfo.Parser.Name, ChecksumIndex: S3Location{Bucket: bucket.Name, Path: idx.Path(fetch, rootPath)}, S3Content: S3Location{Bucket: bucket.Name, Path: contentPath}, ParserData: fetch.urlInfo.Parser}

		}

		// Here goes alternate index managers.
		for _, index := range indexes {
			if index.Name == "demo_index_dead_code" {
				// Implementation of the new index in a similar fashion to the code above.
				// Use bucket.Get to grab the index, check for an error. If any, index file
				// is new so use bucket.Put with the index.Path and index.Content to write data.
				// Else, append index.Content to the current data and bucket.Put to index.Path. Voila!
			}
		}
		wg.Done()
	}
}

// LogFetches processes all the Fetch items are writes the log to S3 for the parsers to start working.
func LogFetches(logChan <-chan *Fetch, errChan <-chan *FetchError, duration *time.Duration) {
	report := &Report{Novel: 0, Errors: 0, Total: 0}
	fetchDuration := &FetchDuration{Hours: duration.Hours(), Minutes: duration.Minutes(), Seconds: duration.Seconds()}
	fetches := &Fetches{}
	for fetch := range logChan {
		fetches.Fetch = append(fetches.Fetch, fetch)
		if fetch.Novel {
			report.Novel++
		}
		report.Total++
	}

	for err := range errChan {
		fetches.FetchError = append(fetches.FetchError, err)
		report.Errors++
		report.Total++
	}

	fetches.Meta = &Meta{FetchDuration: fetchDuration, Report: report}

	content, _ := xml.MarshalIndent(fetches, "", "\t")

	// Write to S3.
	logPath := logFilePath()
	s3content := []byte(xml.Header + string(content))
	S3BucketFromOS().Put(logPath, s3content, "application/xml", s3.Private)
}

func logFilePath() string {
	rootPath := "/gofetch"
	if testGofetch {
		rootPath += "/test_data"
	}
	return fmt.Sprintf("%s/log/%s_%s_%s_%s.xml", rootPath, time.Now().Format("2006-01-02"), os.Getenv("FETCH_ID"), os.Getenv("FETCH_OFFSET"), os.Getenv("FETCH_LIMIT"))
}
