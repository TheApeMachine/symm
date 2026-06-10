package audit

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestRecorderWriteRejectsNonFiniteFloat(t *testing.T) {
	convey.Convey("Given an audit recorder", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)

		convey.Convey("It should reject non-finite floats", func() {
			convey.So(err, convey.ShouldBeNil)

			writeErr := recorder.Write(map[string]any{
				"channel": "ui",
				"type":    "gauge",
				"value": map[string]any{
					"confidence": math.NaN(),
					"surprise":   math.Inf(1),
					"samples":    10,
				},
			})

			convey.So(writeErr, convey.ShouldNotBeNil)
			convey.So(recorder.Close(), convey.ShouldBeNil)
		})
	})
}

func TestRecorderConcurrentWrite(t *testing.T) {
	convey.Convey("Given concurrent audit writes", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)

		convey.Convey("It should produce one valid json object per line", func() {
			convey.So(err, convey.ShouldBeNil)

			const writers = 32
			const linesPerWriter = 64

			waitGroup := sync.WaitGroup{}
			waitGroup.Add(writers)

			for writerIndex := range writers {
				go func(index int) {
					defer waitGroup.Done()

					for lineIndex := range linesPerWriter {
						_ = recorder.Write(map[string]any{
							"writer": index,
							"line":   lineIndex,
						})
					}
				}(writerIndex)
			}

			waitGroup.Wait()
			convey.So(recorder.Close(), convey.ShouldBeNil)

			file, openErr := os.Open(path)
			convey.So(openErr, convey.ShouldBeNil)

			scanner := bufio.NewScanner(file)
			lineCount := 0

			for scanner.Scan() {
				var decoded map[string]any
				convey.So(json.Unmarshal(scanner.Bytes(), &decoded), convey.ShouldBeNil)
				lineCount++
			}

			convey.So(scanner.Err(), convey.ShouldBeNil)
			convey.So(lineCount, convey.ShouldEqual, writers*linesPerWriter)
			convey.So(file.Close(), convey.ShouldBeNil)
		})
	})
}

func BenchmarkRecorderWrite(b *testing.B) {
	path := b.TempDir() + "/audit.jsonl"
	recorder, err := NewRecorder(path)

	if err != nil {
		b.Fatal(err)
	}

	event := map[string]any{
		"channel": "measurements",
		"type":    "measurements",
		"value": map[string]any{
			"Source":     "depthflow",
			"Symbol":     "ETH/USD",
			"Price":      1608.695,
			"Strength":   0.2453732233965164,
			"Volume":     0.0,
			"Spread":     0.45,
			"Confidence": 0.8,
			"Surprise":   1.2,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := recorder.Write(event); err != nil {
			b.Fatal(err)
		}
	}

	if err := recorder.Close(); err != nil {
		b.Fatal(err)
	}
}
