package main

import (
	"fmt"
)

// IndexInterface details what can be considered an index interface.
type IndexInterface interface {
	Path(*HTTPFetch, string) string    // Returns the path on S3 from the HTTPFetch and the root path.
	Content(*HTTPFetch, string) string // Returns the content to store in the index from the HTTPFetch and the path to the content.
}

// CanonicalIndex is the canonical SHA384 index, which cannot be disabled.
type CanonicalIndex struct {
}

// Path returns the
func (idx CanonicalIndex) Path(fetch *HTTPFetch, root string) string {
	return fmt.Sprintf("%s/index/sha384_checksum/%s", root, fetch.checksum)
}

// Content on ChecksumIndex returns the link, the time of the fetch, the duration of the fetch and the parser for that fetch.
func (idx CanonicalIndex) Content(fetch *HTTPFetch, contentPath string) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n", contentPath, fetch.urlInfo.Link, fetch.response.Request.URL.RequestURI(),
		fetch.startTime.Format("2006-01-02T15:04:05.000Z"), fetch.duration, fetch.urlInfo.Parser.Name)
}
