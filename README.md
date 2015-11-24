# Gofetch
## Purpose
Fetch a set of URLs to store and index them on AWS S3. This allows to have fetchers and scrapers siloed from the rest of the infrastructure. Below is described the input and output documents of the fetchers and scrapers, as they operate isolated from any database and exclusively use Amazon S3 as a data store.

### Fetcher
#### Input
##### Fetching input
* URL: the URL to be scraped.
* Processor meta information:
  * Processor name (e.g. “Pubmed_XML”, “Pubmed_HTML”, “RSS”, “Wiley_HTML”)
  * Additional processor specific information (e.g. source name for RSS)

##### Program input
* Configuration file location (which contains additional configuration)
* Scraping URL range (e.g. this program must scrape items x to y from the list of URLs)
* Number of CPUs to run on
* Scrape unique ID (used for output logging information). This can be generated from shell as such `FETCH_ID=$(cat /dev/urandom | tr -dc '0-9' | fold -w 5 | head -n 1)`.

##### Configuration file
Please refer to the example XML files since those will be the most up to date.

#### Output
The output consists of multiple types of files:
* The scraped data as is.
* The log of the scrape.
* One of more index files.

##### Scraped data
* Stored in `{AWS_BUCKET}/{PROGRAM_NAME}/sha384_content/{CONTENT_CHECKSUM}`
* No change is done to the content. The checksum allows to uniquely identify the content. It is a SHA-384 checksum (from SHA-2). Selection was based on language and library availability, the theoretical existence of attacks on SHA-1, the recommendation done to US federal agencies and the computational speed (only slightly slower than SHA-1, whereas SHA-256 is much slower).

##### Scrape log
* Stored in `{AWS_BUCKET}/{PROGRAM_NAME}/logs/{DATE[yyyy-mm-dd]}_{UNIQUE_ID}_{OFFSET}_{LIMIT}`
  * Unique_ID is an ID of the fetch determined by the scheduler.
  * The offset is the starting point from the list of URLs as determined by the scheduler.
  * The limit is the max number of URLs fetched by a given instance as requested by the scheduler.
* It is to be consumed by the processors.

## Configuration
### Environment variables
Environment variable marked with a star are mandatory.
#### AWS_ACCESS_KEY_ID *
AWS access key ID used to communicate with AWS.
#### AWS_SECRET_ACCESS_KEY *
AWS secret key used to communicate with AWS.
#### AWS_STORAGE_BUCKET_NAME *
AWS bucket name used to read and store fetched content from/on AWS.
#### AWS_CONFIG_FILE *
Path to configuration file for the fetcher on AWS.
#### FETCH_ID *
Unique ID representing this fetch.
#### FETCH_OFFSET *
The offset of the URL to fetch with, e.g. `0` to start from the very beginning of the list of URLs, or `50` to start with the fiftieth URL.
#### FETCH_LIMIT *
The maximum number of URLs to fetch, starting from `FETCH_OFFSET`, e.g. `50` to fetches URLs `{FETCH_OFFSET}` to `50+{FETCH_OFFSET}`.
#### REDIS_URL *
*Note:* This is used only by the push2redis script.
The Redis URL where to Push2Redis should push the feeds to be parsed to.
#### CONCURRENT_FETCHES
Number of fetches to run concurrently per CPU. **Default:** 25.
#### CONCURRENT_S3WRITERS
Number of S3 writers to run concurrently. **Default:** 4.
#### MAX_CPUS
Used to determine how many CPUs the fetcher should run on (i.e. pure parallelism). **Default:** number of CPUs on the machine.
#### LOG_LEVEL
Used to set the logging level. Accepts any of the values defined in [go-logging](https://github.com/op/go-logging/blob/2a2006aaf4ee5abc6c8b0bd5246982616d621139/level.go#L27). **Default:** INFO.

### Configuration file
The configuration file is in XML, as defined and documented in [docs/config.xsd](docs/config.xsd). It allows enabling and disabling of indexes,
as well as determining the parser names and metadata for the fetched content.

## Output files
### Fetched content
The fetched content is stored on the provided AWS bucket in `/gofetch/sha384_content/` (not configurable to avoid different deployments from writing to different places).
As the _directory_ name implies, the file name corresponds to the [SHA-384](http://en.wikipedia.org/wiki/SHA-2). The choice for SHA-384 over SHA-1 was made given that
the latter has known theoretical attacks, and SHA-384 is only slightly slower to compute than SHA-1 (whereas SHA-256 is noticeably slower).
### Indexes
It is possible to define indexes which store metadata related to the content.
#### Current indexes
##### SHA-384 checksum index
This is the **canonical index**, and hence cannot be disabled through the configuration file. As coded in [indexes.go](indexes.go), the index adds each checksum as its own file
into the `/gofetch/index/sha384_checksum/` _directory_, . Hence, each fetched content is either found in that directory by the SHA-384 (hex encoded) checksum, or added to that directory.
If found, the fetcher will append content the file in the following format. Note that given the possible variety of parser metadata, this information is lost in the index.
Also note that the content location should be the same all the time, but is required for additional indexes to find the content and in case there is a structure change.
```
{content_location}\t{requested_link}\t{final_link}\t{fetch_start_datetime}\t{fetch_duration[nanoseconds]}\t{parser_name}

```
#### Adding new indexes
1. New indexes must implement the `IndexInterface` interface defined in [indexes.go](indexes.go).
2. The appropriate code logic, which creates the index object and names it, must be added in [s3mgr.go](s3mgr.go).
2. This README.md file must contain the appropriate documentation.
