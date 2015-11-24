package main

import (
	"encoding/xml"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"strings"
	"testing"
)

// TestGofetch tests all of GoFetch features with dummy datasets hosted on S3.
func TestGofetch(t *testing.T) {
	testGofetch = true
	// Setting some environment variables.
	testSettings := map[string]string{"MAX_CPUS": "1", "AWS_STORAGE_BUCKET_NAME": "example-bucket",
		"LOG_LEVEL": "DEBUG", "AWS_CONFIG_FILE": "/gofetch/test_data/test_config_nominal.xml", "FETCH_ID": "1",
		"FETCH_OFFSET": "0", "FETCH_LIMIT": "10"}
	for env, val := range testSettings {
		err := os.Setenv(env, val)
		if err != nil {
			panic(fmt.Errorf("could not set %s to %s", env, val))
		}
		log.Debug("Set envvar %s to %s.", env, val)
	}

	Convey("With dummy data, check that all output is nominal and reset S3 test folder", t, func() {
		bucket := S3BucketFromOS()

		// Expectations
		expChecksumPath := []string{"/gofetch/test_data/index/sha384_checksum/a6cd5b1ffba20c8789d770c3ed80263947c4f13510fb2d63cb1c96043e25f2b4f5b0b40f8f91c21606cec409cb376dfb",
			"/gofetch/test_data/index/sha384_checksum/3c0edeebdfb207f6b233368a06bfd612e00a19afa9ea0468ecad85c9f245c1b5f9c5dff62408d4374fa975aaf3a87b64"}

		expContentPath := []string{"/gofetch/test_data/sha384_content/3c0edeebdfb207f6b233368a06bfd612e00a19afa9ea0468ecad85c9f245c1b5f9c5dff62408d4374fa975aaf3a87b64",
			"/gofetch/test_data/sha384_content/a6cd5b1ffba20c8789d770c3ed80263947c4f13510fb2d63cb1c96043e25f2b4f5b0b40f8f91c21606cec409cb376dfb"}

		expIndexLinks := []string{"http://example-bucket.s3.amazonaws.com/gofetch/test_data/feeds/apa-journals-pas.xml",
			"http://example-bucket.s3.amazonaws.com/gofetch/test_data/feeds/dydan1.xml"}

		logFile := logFilePath()

		defer func() {
			// Let's delete the test data as a defer of this test.
			for pi := range expChecksumPath {
				bucket.Del(expChecksumPath[pi])
			}
			for pi := range expContentPath {
				bucket.Del(expContentPath[pi])
			}
			bucket.Del(logFile)
		}()

		main()
		// Let's grab the log file.
		logBody, notFoundErr := bucket.Get(logFile)
		if notFoundErr != nil {
			panic(notFoundErr)
		}
		log := Fetches{}
		xmlErr := xml.Unmarshal(logBody, &log)
		if xmlErr != nil {
			// Oops, couldn't read the configuration file! Gotta panic now!
			panic(xmlErr)
		}

		So(log.Meta.Report.Novel, ShouldEqual, 2)
		So(log.Meta.Report.Errors, ShouldEqual, 1)
		So(log.Meta.Report.Total, ShouldEqual, 4)
		So(len(log.FetchError), ShouldEqual, 1)
		So(log.FetchError[0].Original, ShouldEqual, "http:/some.invalid.com/link")
		So(log.FetchError[0].Cleaned, ShouldEqual, "http:/some.invalid.com/link")

		for fid := range log.Fetch {
			fetch := log.Fetch[fid]
			So(fetch.Parser, ShouldEqual, "RawArticle")
			So(fetch.ChecksumIndex.Bucket, ShouldEqual, S3BucketFromOS().Name)
			So(fetch.ChecksumIndex.Path, ShouldBeIn, expChecksumPath)
			So(fetch.S3Content.Bucket, ShouldEqual, S3BucketFromOS().Name)
			So(fetch.S3Content.Path, ShouldBeIn, expContentPath)

			// Let's load the index for this item and check its validity.
			idxBody, notFoundErr := bucket.Get(fetch.ChecksumIndex.Path)
			if notFoundErr != nil {
				panic(notFoundErr)
			}
			idxLines := strings.Split(string(idxBody), "\n")
			// The index is always appended with an empty line.
			So(len(idxLines), ShouldBeLessThanOrEqualTo, 3)
			for lno := range idxLines {
				if idxLines[lno] == "" {
					continue // Last line of index.
				}
				rows := strings.Fields(idxLines[lno])
				So(len(rows), ShouldEqual, 6)
				So(rows[0], ShouldBeIn, expContentPath)
				So(rows[1], ShouldBeIn, expIndexLinks)
				So(rows[5], ShouldEqual, "RawArticle")
			}

		}

	})

	Convey("With empty config file", t, func() {
		os.Setenv("AWS_CONFIG_FILE", "/gofetch/test_data/test_config_empty.xml")
		So(main, ShouldPanic)
	})

	Convey("With an invalid throttling duration in config file", t, func() {
		os.Setenv("AWS_CONFIG_FILE", "/gofetch/test_data/test_config_invalid_duration.xml")
		So(main, ShouldPanic)
	})
}
