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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
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
		var contentsXML strings.Builder

		for objKey := range backend.objects {
			if prefix == "" || strings.HasPrefix(objKey, prefix) {
				contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.objects[objKey])))
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

		Convey("When Put is called with a binary Measurement", func() {
			now := time.Now().UTC().Truncate(time.Nanosecond)
			prior := now.Add(-time.Second)

			measurement := data.NewMeasurement(
				1,
				"BTC/USD",
				"spot:trade",
				now,
				prior,
				&data.StringEntry{Key: "feed", Value: "kraken-ws"},
			)

			priceDecimal, err := decimal.NewFromString("92500.25")
			So(err, ShouldBeNil)

			priceMetric := data.NewExactMetric("price", priceDecimal, data.UnitPrice, data.TimescaleInstantaneous)

			measurement.Write(priceMetric)

			putErr := client.Put(measurement)
			So(putErr, ShouldBeNil)

			expectedKey := fmt.Sprintf("1/BTC/USD/spot:trade/%d.bin", measurement.SeqIdx)
			So(len(backend.objects[expectedKey]), ShouldBeGreaterThan, 0)

			Convey("Then Get retrieves and unmarshals the exact measurement", func() {
				var retrieved []*data.Measurement

				for item := range client.Get("1/BTC/USD") {
					retrieved = append(retrieved, item)
				}

				So(len(retrieved), ShouldEqual, 1)
				So(retrieved[0].ID, ShouldEqual, measurement.ID)
				So(retrieved[0].Label, ShouldEqual, "BTC/USD")
				So(retrieved[0].Source, ShouldEqual, "spot:trade")
				So(retrieved[0].Epoch, ShouldEqual, 1)

				readCount := 0
				for entry := range retrieved[0].Read("price") {
					readCount++
					So(entry.Metric.Exact().String(), ShouldEqual, "92500.25")
					So(entry.Metric.Unit(), ShouldEqual, data.UnitPrice)
				}
				So(readCount, ShouldEqual, 1)
			})
		})
	})
}
