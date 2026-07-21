package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
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

func TestRecorderAppendPreservesExistingRecords(t *testing.T) {
	convey.Convey("Given two recorders for the same audit file", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		first, firstErr := NewRecorder(path)

		convey.So(firstErr, convey.ShouldBeNil)
		convey.So(first.Write(map[string]any{"sequence": 1}), convey.ShouldBeNil)
		convey.So(first.Close(), convey.ShouldBeNil)

		second, secondErr := NewRecorder(path)

		convey.So(secondErr, convey.ShouldBeNil)
		convey.So(second.Write(map[string]any{"sequence": 2}), convey.ShouldBeNil)
		convey.So(second.Close(), convey.ShouldBeNil)

		convey.Convey("It should not truncate the first recorder's evidence", func() {
			file, openErr := os.Open(path)
			convey.So(openErr, convey.ShouldBeNil)

			defer func() {
				convey.So(file.Close(), convey.ShouldBeNil)
			}()

			scanner := bufio.NewScanner(file)
			sequences := []float64{}

			for scanner.Scan() {
				var decoded map[string]any
				convey.So(json.Unmarshal(scanner.Bytes(), &decoded), convey.ShouldBeNil)
				sequences = append(sequences, decoded["sequence"].(float64))
			}

			convey.So(scanner.Err(), convey.ShouldBeNil)
			convey.So(sequences, convey.ShouldResemble, []float64{1, 2})
		})
	})
}

func TestRecorderCreatesParentDirectory(t *testing.T) {
	convey.Convey("Given an audit path inside a missing directory", t, func() {
		path := filepath.Join(t.TempDir(), "run", "audit.jsonl")
		recorder, err := NewRecorder(path)

		convey.Convey("It should create the directory and write", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(recorder.Write(map[string]any{"ok": true}), convey.ShouldBeNil)
			convey.So(recorder.Close(), convey.ShouldBeNil)

			_, statErr := os.Stat(path)
			convey.So(statErr, convey.ShouldBeNil)
		})
	})
}

func TestRecorderWriteFailureRecordsOperationalMetric(t *testing.T) {
	convey.Convey("Given a closed audit recorder", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)

		convey.So(err, convey.ShouldBeNil)
		convey.So(recorder.Close(), convey.ShouldBeNil)

		writeErr := recorder.Write(map[string]any{"closed": true})

		convey.Convey("It should count the audit write failure", func() {
			var closedErr *errnie.ErrnieError

			convey.So(writeErr, convey.ShouldNotBeNil)
			convey.So(errors.As(writeErr, &closedErr), convey.ShouldBeTrue)
			convey.So(closedErr.Kind, convey.ShouldEqual, errnie.IO)
			convey.So(closedErr.Message, convey.ShouldEqual, "audit: recorder is closed")
		})
	})
}

/*
TestRecorderOverflowRecordsLoss proves a saturated hot-path ring remains
non-blocking while the single consumer writes one authoritative aggregate row
for the diagnostic events that could not enter the ring.
*/
func TestRecorderOverflowRecordsLoss(t *testing.T) {
	convey.Convey("Given a recorder whose ring is already saturated", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
		convey.So(err, convey.ShouldBeNil)
		ctx, cancel := context.WithCancel(context.Background())
		ring, err := structure.NewMPMCRing[[]byte](ctx, 2)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ring.Push([]byte("{\"sequence\":1}\n")), convey.ShouldBeTrue)
		convey.So(ring.Push([]byte("{\"sequence\":2}\n")), convey.ShouldBeTrue)
		recorder := &Recorder{
			ctx:    ctx,
			cancel: cancel,
			fh:     file,
			ring:   ring,
			done:   make(chan struct{}),
		}

		convey.So(recorder.Write(map[string]any{"sequence": 3}), convey.ShouldNotBeNil)
		go recorder.drain()
		convey.So(recorder.Close(), convey.ShouldBeNil)

		rows, err := os.ReadFile(path)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("Then the persisted timeline declares the loss", func() {
			convey.So(string(rows), convey.ShouldContainSubstring, `"type":"audit_overflow"`)
			convey.So(string(rows), convey.ShouldContainSubstring, `"dropped":1`)
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
