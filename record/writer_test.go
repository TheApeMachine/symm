package record

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestWriterWriteAndTick(t *testing.T) {
	convey.Convey("Given a record writer", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		viper.Set("trading.record.file", path)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writer, err := NewWriter(ctx)

		convey.Convey("It should flush queued frames on Tick", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(writer, convey.ShouldNotBeNil)

			convey.So(writer.Write("measurement", map[string]any{"symbol": "BTC/USD"}), convey.ShouldBeNil)
			convey.So(writer.Tick(), convey.ShouldBeNil)
			convey.So(writer.Close(), convey.ShouldBeNil)

			file, openErr := os.Open(path)
			convey.So(openErr, convey.ShouldBeNil)

			scanner := bufio.NewScanner(file)
			convey.So(scanner.Scan(), convey.ShouldBeTrue)

			var frame map[string]any
			convey.So(json.Unmarshal(scanner.Bytes(), &frame), convey.ShouldBeNil)
			convey.So(frame["type"], convey.ShouldEqual, "measurement")
			convey.So(file.Close(), convey.ShouldBeNil)
		})
	})
}

func TestWriterCloseDrainsQueue(t *testing.T) {
	convey.Convey("Given a record writer with pending jobs", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		viper.Set("trading.record.file", path)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writer, err := NewWriter(ctx)

		convey.Convey("It should drain the queue on Close", func() {
			convey.So(err, convey.ShouldBeNil)

			convey.So(writer.Write("first", 1), convey.ShouldBeNil)
			convey.So(writer.Write("second", 2), convey.ShouldBeNil)
			convey.So(writer.Close(), convey.ShouldBeNil)

			file, openErr := os.Open(path)
			convey.So(openErr, convey.ShouldBeNil)

			scanner := bufio.NewScanner(file)
			lineCount := 0

			for scanner.Scan() {
				lineCount++
			}

			convey.So(scanner.Err(), convey.ShouldBeNil)
			convey.So(lineCount, convey.ShouldEqual, 2)
			convey.So(file.Close(), convey.ShouldBeNil)
		})
	})
}

func TestWriterDropsWhenQueueFull(t *testing.T) {
	convey.Convey("Given a record writer with a full queue", t, func() {
		path := filepath.Join(t.TempDir(), "capture.jsonl")
		viper.Set("trading.record.file", path)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writer, err := NewWriter(ctx)

		convey.Convey("It should count drops instead of blocking producers", func() {
			convey.So(err, convey.ShouldBeNil)

			writer.queue = make(chan recordJob, 1)
			convey.So(writer.Write("first", 1), convey.ShouldBeNil)
			convey.So(writer.Write("second", 2), convey.ShouldBeNil)
			convey.So(writer.Drops(), convey.ShouldEqual, 1)
			convey.So(writer.Close(), convey.ShouldBeNil)
		})
	})
}

func TestNewWriterNilWhenPathUnset(t *testing.T) {
	convey.Convey("Given no capture path", t, func() {
		viper.Set("trading.record.file", "")

		writer, err := NewWriter(context.Background())

		convey.Convey("It should return nil without error", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(writer, convey.ShouldBeNil)
		})
	})
}

func BenchmarkWriterWrite(b *testing.B) {
	path := b.TempDir() + "/capture.jsonl"
	viper.Set("trading.record.file", path)

	ctx := context.Background()
	writer, err := NewWriter(ctx)

	if err != nil {
		b.Fatal(err)
	}

	event := map[string]any{
		"symbol":     "ETH/USD",
		"confidence": 0.8,
		"surprise":   1.2,
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := writer.Write("measurement", event); err != nil {
			b.Fatal(err)
		}
	}

	if err := writer.Tick(); err != nil {
		b.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
}
