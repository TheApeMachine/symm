package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/s3"
	"gocloud.dev/gcerrors"
)

func TestNewS3(t *testing.T) {
	Convey("Opening S3 from the loaded configuration", t, func() {
		viper.Reset()
		t.Cleanup(viper.Reset)

		Convey("Missing configuration returns an error", func() {
			client, err := NewS3(context.Background())
			So(err, ShouldNotBeNil)
			So(client, ShouldBeNil)
		})

		Convey("Datura rejects conflicting connection settings", func() {
			viper.Set("storage.s3.bucket_url", "s3://test")
			viper.Set("storage.s3.bucket", "another-bucket")
			client, err := NewS3(context.Background())
			So(err, ShouldNotBeNil)
			So(client, ShouldBeNil)
		})

		Convey("The real client writes and reads exact bytes over HTTP", func() {
			client := newS3Fixture(t, 0)
			ctx := context.Background()

			for _, payload := range [][]byte{{0, 255, 1, 0}, []byte("replacement"), {}} {
				So(client.Bucket().WriteAll(ctx, "capture", payload, nil), ShouldBeNil)
				actual, err := client.Bucket().ReadAll(ctx, "capture")
				So(err, ShouldBeNil)
				So(bytes.Equal(actual, payload), ShouldBeTrue)
			}

			Convey("A reopened client reads the stored object", func() {
				So(client.Bucket().WriteAll(ctx, "capture", []byte("saved"), nil), ShouldBeNil)
				reopened, err := NewS3(ctx)
				So(err, ShouldBeNil)
				actual, err := reopened.Bucket().ReadAll(ctx, "capture")
				So(err, ShouldBeNil)
				So(string(actual), ShouldEqual, "saved")
				So(reopened.Close(), ShouldBeNil)
			})

			Convey("Missing objects remain distinguishable from stored empty data", func() {
				_, err := client.Bucket().ReadAll(ctx, "missing")
				So(gcerrors.Code(err), ShouldEqual, gcerrors.NotFound)
			})

			Convey("Cancellation is returned to the caller", func() {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				So(client.Bucket().WriteAll(cancelled, "capture", []byte("cancelled"), nil), ShouldNotBeNil)
				_, err := client.Bucket().ReadAll(cancelled, "capture")
				So(err, ShouldNotBeNil)
			})
		})
	})
}

// newS3Fixture exercises Datura and the S3 SDK against a local HTTP fixture.
// It is not verification of a deployed S3 service.
func newS3Fixture(t testing.TB, interruptions int) *s3.Client {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	objects := make(map[string][]byte)
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()

		if !strings.HasPrefix(request.URL.Path, "/test/symm/") {
			http.Error(response, "configured bucket or prefix missing", http.StatusBadRequest)
			return
		}

		if request.Method == http.MethodPut {
			payload, err := io.ReadAll(request.Body)

			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}

			objects[request.URL.Path] = payload
			return
		}

		payload, found := objects[request.URL.Path]

		if !found {
			http.Error(response, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}

		response.Header().Set("Content-Length", fmt.Sprint(len(payload)))

		if interruptions > 0 {
			interruptions--
			if _, err := response.Write(payload[:len(payload)/2]); err != nil {
				t.Errorf("serve interrupted body: %v", err)
			}
			return
		}

		checksum := sha256.Sum256(payload)
		response.Header().Set("x-amz-checksum-sha256", base64.StdEncoding.EncodeToString(checksum[:]))

		if _, err := response.Write(payload); err != nil {
			t.Errorf("serve S3 object: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(
		"storage:\n  s3:\n    bucket_url: s3://test?region=us-east-1&endpoint=%s&use_path_style=true\n    prefix: symm/\n",
		server.URL,
	)))

	if err != nil {
		t.Fatal(err)
	}

	client, err := NewS3(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close S3 client: %v", err)
		}
	})

	return client
}

func BenchmarkNewS3(b *testing.B) {
	client := newS3Fixture(b, 0)
	ctx := context.Background()
	// A 64 KiB binary object models raw capture data; no market threshold is used.
	payload := bytes.Repeat([]byte{0, 255, 1, 0}, 16384)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := client.Bucket().WriteAll(ctx, "capture", payload, nil); err != nil {
			b.Fatal(err)
		}

		actual, err := client.Bucket().ReadAll(ctx, "capture")

		if err != nil {
			b.Fatal(err)
		}

		if !bytes.Equal(actual, payload) {
			b.Fatal("object bytes changed")
		}
	}
}

func TestReadAll(t *testing.T) {
	Convey("Interrupted S3 response bodies are retried before decoding", t, func() {
		for _, interruptions := range []int{1, 2, 3} {
			Convey(fmt.Sprintf("With %d truncated downloads", interruptions), func() {
				client := newS3Fixture(t, interruptions)
				payload := []byte(`{"original":"complete capture"}`)
				So(client.Bucket().WriteAll(t.Context(), "capture", payload, nil), ShouldBeNil)
				actual, err := ReadAll(t.Context(), client.Bucket(), "capture")

				if interruptions < retry.NewStandard().MaxAttempts() {
					So(err, ShouldBeNil)
					So(actual, ShouldResemble, payload)
					return
				}
				So(errors.Is(err, io.ErrUnexpectedEOF), ShouldBeTrue)
				So(actual, ShouldBeNil)
			})
		}
		Convey("Missing keys and cancellation retain their original meaning", func() {
			client := newS3Fixture(t, 0)
			_, err := ReadAll(t.Context(), client.Bucket(), "missing")
			So(gcerrors.Code(err), ShouldEqual, gcerrors.NotFound)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err = ReadAll(ctx, client.Bucket(), "capture")
			So(errors.Is(err, context.Canceled), ShouldBeTrue)
		})
	})
}

func BenchmarkReadAll(b *testing.B) {
	client := newS3Fixture(b, 0)
	payload := bytes.Repeat([]byte("capture"), 8192) // A 56KiB capture object.

	if err := client.Bucket().WriteAll(b.Context(), "capture", payload, nil); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		actual, err := ReadAll(b.Context(), client.Bucket(), "capture")

		if err != nil {
			b.Fatal(err)
		}

		if !bytes.Equal(actual, payload) {
			b.Fatal("download bytes changed")
		}
	}
}
