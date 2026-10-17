/*
Package storetest is an in-memory S3 for unit tests: no test may reach the
real bucket. Serve a Backend with httptest and pass Options(server.URL) to
store.NewClient.
*/
package storetest

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

/*
Backend answers the S3 calls store.Client makes (bucket create/head,
ListObjectsV2, PutObject, GetObject) from memory.
*/
type Backend struct {
	Mu      sync.Mutex
	Buckets map[string]bool
	Objects map[string][]byte
}

func NewBackend() *Backend {
	return &Backend{
		Buckets: make(map[string]bool),
		Objects: make(map[string][]byte),
	}
}

func (backend *Backend) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	backend.Mu.Lock()
	defer backend.Mu.Unlock()

	path := strings.TrimPrefix(request.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	bucketName := parts[0]
	key := ""

	if len(parts) == 2 {
		key = parts[1]
	}

	if request.Method == http.MethodHead && key == "" {
		if backend.Buckets[bucketName] {
			writer.WriteHeader(http.StatusOK)
			return
		}

		http.Error(writer, "Not found", http.StatusNotFound)
		return
	}

	if request.Method == http.MethodPut && key == "" {
		backend.Buckets[bucketName] = true
		writer.WriteHeader(http.StatusOK)
		return
	}

	// 1. ListObjectsV2
	if request.Method == http.MethodGet && request.URL.Query().Get("list-type") == "2" {
		prefix := request.URL.Query().Get("prefix")
		delimiter := request.URL.Query().Get("delimiter")
		var contentsXML strings.Builder

		if delimiter != "" {
			seenPrefixes := make(map[string]bool)

			for objKey := range backend.Objects {
				if prefix == "" || strings.HasPrefix(objKey, prefix) {
					sub := strings.TrimPrefix(objKey, prefix)
					idx := strings.Index(sub, delimiter)

					if idx >= 0 {
						commonPrefix := prefix + sub[:idx+len(delimiter)]

						if !seenPrefixes[commonPrefix] {
							seenPrefixes[commonPrefix] = true
							contentsXML.WriteString(fmt.Sprintf("<CommonPrefixes><Prefix>%s</Prefix></CommonPrefixes>", commonPrefix))
						}
					}

					if idx < 0 {
						contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.Objects[objKey])))
					}
				}
			}
		}

		if delimiter == "" {
			for objKey := range backend.Objects {
				if prefix == "" || strings.HasPrefix(objKey, prefix) {
					contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.Objects[objKey])))
				}
			}
		}

		xmlResponse := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>%s</Name>
    <Prefix>%s</Prefix>
    <KeyCount>1</KeyCount>
    <MaxKeys>1000</MaxKeys>
    <IsTruncated>false</IsTruncated>
    %s
</ListBucketResult>`, parts[0], prefix, contentsXML.String())

		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(xmlResponse))
		return
	}

	// 2. PutObject
	if request.Method == http.MethodPut {
		body, err := io.ReadAll(request.Body)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		backend.Objects[key] = body
		writer.WriteHeader(http.StatusOK)
		return
	}

	// 3. GetObject
	if request.Method == http.MethodGet {
		payload, exists := backend.Objects[key]

		if !exists {
			http.Error(writer, "Not found", http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", "application/octet-stream")

		// A ranged GET (the SDK's download manager asks for parts) is
		// answered as S3 does: 206 with Content-Range, so the manager knows
		// the object's size instead of asking again.
		if spec := request.Header.Get("Range"); spec != "" {
			var first, last int

			if _, err := fmt.Sscanf(spec, "bytes=%d-%d", &first, &last); err != nil || first < 0 || first > last {
				http.Error(writer, "unsupported range "+spec, http.StatusRequestedRangeNotSatisfiable)
				return
			}

			if first >= len(payload) {
				writer.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(payload)))
				http.Error(writer, "range beyond the object", http.StatusRequestedRangeNotSatisfiable)
				return
			}

			last = min(last, len(payload)-1)
			writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(payload)))
			writer.Header().Set("Content-Length", strconv.Itoa(last-first+1))
			writer.WriteHeader(http.StatusPartialContent)
			writer.Write(payload[first : last+1])
			return
		}

		writer.WriteHeader(http.StatusOK)
		writer.Write(payload)
		return
	}

	http.NotFound(writer, request)
}

/*
Options points an S3 client at a Backend served at url, with test
credentials and path-style addressing.
*/
func Options(url string) func(*s3.Options) {
	return func(options *s3.Options) {
		options.BaseEndpoint = aws.String(url)
		options.UsePathStyle = true
		options.Region = "us-east-1"
		options.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
	}
}
