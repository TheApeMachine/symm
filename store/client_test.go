package store

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	. "github.com/smartystreets/goconvey/convey"
)

type mockS3Backend struct {
	mu      sync.Mutex
	buckets map[string]bool
	objects map[string][]byte
}

func newMockS3Backend() *mockS3Backend {
	return &mockS3Backend{
		buckets: make(map[string]bool),
		objects: make(map[string][]byte),
	}
}

func (backend *mockS3Backend) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	path := strings.TrimPrefix(request.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	bucketName := parts[0]
	key := ""

	if len(parts) == 2 {
		key = parts[1]
	}

	if request.Method == http.MethodHead && key == "" {
		if backend.buckets[bucketName] {
			writer.WriteHeader(http.StatusOK)
			return
		}

		http.Error(writer, "Not found", http.StatusNotFound)
		return
	}

	if request.Method == http.MethodPut && key == "" {
		backend.buckets[bucketName] = true
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

			for objKey := range backend.objects {
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
						contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.objects[objKey])))
					}
				}
			}
		}

		if delimiter == "" {
			for objKey := range backend.objects {
				if prefix == "" || strings.HasPrefix(objKey, prefix) {
					contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.objects[objKey])))
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

		backend.objects[key] = body
		writer.WriteHeader(http.StatusOK)
		return
	}

	// 3. GetObject
	if request.Method == http.MethodGet {
		payload, exists := backend.objects[key]

		if !exists {
			http.Error(writer, "Not found", http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.WriteHeader(http.StatusOK)
		writer.Write(payload)
		return
	}

	http.NotFound(writer, request)
}

func TestStoreClient(t *testing.T) {
	Convey("Given an S3 store Client configured with mock backend", t, func() {
		backend := newMockS3Backend()
		server := httptest.NewServer(backend)
		defer server.Close()

		ctx := context.Background()
		bucket := "test-bucket"

		client := NewClient(ctx, bucket, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(server.URL)
			options.UsePathStyle = true
			options.Region = "us-east-1"
			options.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
		})

		So(client, ShouldNotBeNil)

		Convey("NewClient ensures bucket is created", func() {
			backend.mu.Lock()
			So(backend.buckets[bucket], ShouldBeTrue)
			backend.mu.Unlock()
		})

		Convey("When Put is called with a key and payload", func() {
			key := "run/epoch/0000000000000000001/0000000000000000010.bin"
			payload := []byte("test-payload-bytes")

			putErr := client.Put(key, payload)
			So(putErr, ShouldBeNil)
			So(string(backend.objects[key]), ShouldEqual, string(payload))

			Convey("Then Download retrieves the exact payload", func() {
				retrieved, getErr := client.Download(key)
				So(getErr, ShouldBeNil)
				So(string(retrieved), ShouldEqual, string(payload))
			})

			Convey("Then GetBytes streams all payloads under prefix", func() {
				var payloads [][]byte
				for p := range client.GetBytes("run/epoch/0000000000000000001/") {
					payloads = append(payloads, p)
				}

				So(len(payloads), ShouldEqual, 1)
				So(string(payloads[0]), ShouldEqual, string(payload))
			})

			Convey("Then List with delimiter returns top-level common prefixes", func() {
				var commonPrefixes []string

				for page := range client.List("run/epoch/", "/") {
					for _, commonPrefix := range page.CommonPrefixes {
						if commonPrefix.Prefix != nil {
							commonPrefixes = append(commonPrefixes, *commonPrefix.Prefix)
						}
					}
				}

				So(len(commonPrefixes), ShouldEqual, 1)
				So(commonPrefixes[0], ShouldEqual, "run/epoch/0000000000000000001/")
			})

			Convey("Then ListKeys returns objects sorted by Epoch, Tick, and SeqIdx", func() {
				keyTick2 := "cvd/BTC/USD/0000000000000000001/0000000000000000002/0000000000000000001/0000000001000000000.bin"
				keyTick1Seq2 := "cvd/BTC/USD/0000000000000000001/0000000000000000001/0000000000000000002/0000000001000000000.bin"
				keyTick1Seq1 := "correlation/BTC/USD/0000000000000000001/0000000000000000001/0000000000000000001/0000000001000000000.bin"

				_ = client.Put(keyTick2, []byte("tick2"))
				_ = client.Put(keyTick1Seq2, []byte("tick1seq2"))
				_ = client.Put(keyTick1Seq1, []byte("tick1seq1"))

				keys := client.ListKeys("correlation/", "cvd/")
				So(len(keys), ShouldEqual, 3)
				So(keys[0], ShouldEqual, keyTick1Seq1)
				So(keys[1], ShouldEqual, keyTick1Seq2)
				So(keys[2], ShouldEqual, keyTick2)
			})
		})
	})
}
