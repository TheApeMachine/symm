package backtest

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureWriterCapture(t *testing.T) {
	Convey("Given public, private, and level3 transports sharing one live capture", t, func() {
		store, err := NewStore(filepath.Join(t.TempDir(), "symm.sqlite"))
		So(err, ShouldBeNil)
		defer func() { So(store.Close(), ShouldBeNil) }()
		writer, err := store.OpenCapture()
		So(err, ShouldBeNil)
		var writers sync.WaitGroup
		endpoints := []string{"public", "private", "level3"}
		writeErrors := make(chan error, len(endpoints)*captureBatchSize)

		for _, endpoint := range endpoints {
			writers.Add(1)

			go func() {
				defer writers.Done()

				for index := range captureBatchSize {
					writeErrors <- writer.Capture(
						endpoint,
						[]byte(endpoint),
						time.Unix(0, int64(index)).UTC(),
					)
				}
			}()
		}

		writers.Wait()
		close(writeErrors)

		for err := range writeErrors {
			So(err, ShouldBeNil)
		}

		So(writer.Close(), ShouldBeNil)
		captures, err := store.ListCaptures()
		So(err, ShouldBeNil)

		Convey("It should retain every frame in the single ordered capture", func() {
			So(captures, ShouldHaveLength, 1)
			So(captures[0].Frames, ShouldEqual, int64(len(endpoints)*captureBatchSize))
			So(captures[0].EndedAt, ShouldNotBeNil)
		})
	})
}

func BenchmarkCaptureWriterCapture(b *testing.B) {
	store, err := NewStore(filepath.Join(b.TempDir(), "symm.sqlite"))

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}()

	writer, err := store.OpenCapture()

	if err != nil {
		b.Fatal(err)
	}

	payload := []byte("captured websocket frame")
	at := time.Unix(0, 1).UTC()
	b.ResetTimer()

	for range b.N {
		if err := writer.Capture("public", payload, at); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
}
