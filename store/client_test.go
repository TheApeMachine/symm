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
	"github.com/theapemachine/symm/data"
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

		Convey("When a Value is written via io.Copy", func() {
			val := data.NewValue("testorigin", "tick", "agg")
			val.Tick = 1
			val.SeqIdx = 10
			val.FormatPrefix()
			val.Append([]byte("test-payload-bytes"))

			n, writeErr := io.Copy(client, val)
			So(writeErr, ShouldBeNil)
			So(n, ShouldBeGreaterThan, 0)
			So(len(backend.objects[val.Prefix]), ShouldBeGreaterThan, 0)

			Convey("Then Download retrieves the exact payload bytes", func() {
				retrieved, getErr := client.Download(val.Prefix)
				So(getErr, ShouldBeNil)
				outVal := data.From(retrieved)
				So(outVal.Origin, ShouldEqual, "testorigin")
				So(len(outVal.Layers), ShouldEqual, 1)
				So(string(outVal.Layers[0]), ShouldEqual, "test-payload-bytes")
			})

			Convey("Then io.Copy from client reads the Value", func() {
				client.prefix = "testorigin/"
				client.keys = nil
				client.cursor = 0
				readVal := data.New()
				rn, rErr := io.Copy(readVal, client)
				So(rErr, ShouldBeNil)
				So(rn, ShouldBeGreaterThan, 0)
				So(readVal.Origin, ShouldEqual, "testorigin")
				So(string(readVal.Layers[0]), ShouldEqual, "test-payload-bytes")
			})

			Convey("Then ListKeys returns objects sorted by Epoch, Tick, and SeqIdx", func() {
				v1 := data.NewValue("cvd", "BTC", "USD")
				v1.Tick = 2
				v1.SeqIdx = 1
				v1.FormatPrefix()
				_, _ = io.Copy(client, v1)

				v2 := data.NewValue("cvd", "BTC", "USD")
				v2.Tick = 1
				v2.SeqIdx = 2
				v2.FormatPrefix()
				_, _ = io.Copy(client, v2)

				v3 := data.NewValue("correlation", "BTC", "USD")
				v3.Tick = 1
				v3.SeqIdx = 1
				v3.FormatPrefix()
				_, _ = io.Copy(client, v3)

				keys := client.ListKeys("correlation/", "cvd/")
				So(len(keys), ShouldEqual, 3)
				So(keys[0], ShouldEqual, v3.Prefix)
				So(keys[1], ShouldEqual, v2.Prefix)
				So(keys[2], ShouldEqual, v1.Prefix)
			})
		})
	})
}
