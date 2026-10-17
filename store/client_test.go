package store

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store/storetest"
)

func TestStoreClient(t *testing.T) {
	Convey("Given an S3 store Client configured with mock backend", t, func() {
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()

		ctx := context.Background()
		bucket := "test-bucket"

		client := NewClient(ctx, bucket, storetest.Options(server.URL))

		So(client, ShouldNotBeNil)

		Convey("NewClient ensures bucket is created", func() {
			backend.Mu.Lock()
			So(backend.Buckets[bucket], ShouldBeTrue)
			backend.Mu.Unlock()
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
			// The upload runs async; wait for it to land on the backend.
			stored := func() bool {
				backend.Mu.Lock()
				defer backend.Mu.Unlock()
				return len(backend.Objects[val.Prefix]) > 0
			}

			for deadline := time.Now().Add(5 * time.Second); !stored() && time.Now().Before(deadline); {
				time.Sleep(5 * time.Millisecond)
			}

			So(stored(), ShouldBeTrue)

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

				// Uploads are async; wait until all three have landed.
				keys, err := client.ListKeys("correlation/", "cvd/")
				So(err, ShouldBeNil)
				for deadline := time.Now().Add(5 * time.Second); len(keys) < 3 && time.Now().Before(deadline); {
					time.Sleep(5 * time.Millisecond)
					keys, err = client.ListKeys("correlation/", "cvd/")
					So(err, ShouldBeNil)
				}

				So(len(keys), ShouldEqual, 3)
				So(keys[0], ShouldEqual, v3.Prefix)
				So(keys[1], ShouldEqual, v2.Prefix)
				So(keys[2], ShouldEqual, v1.Prefix)
			})
		})
	})
}
