package io

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestCountMeasurementLines(t *testing.T) {
	convey.Convey("Given a valid measurement JSONL file", t, func() {
		path := filepath.Join(t.TempDir(), "measurements.jsonl")
		raw := `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":1,"Last":100}` + "\n"
		raw += `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":101}` + "\n"

		writeErr := os.WriteFile(path, []byte(raw), 0o644)
		total, skipped, err := CountMeasurementLines(path)

		convey.Convey("It should count valid rows", func() {
			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
			convey.So(total, convey.ShouldEqual, 2)
		})
	})
}

func TestLoadMeasurementsMalformedLine(t *testing.T) {
	convey.Convey("Given a malformed JSONL line", t, func() {
		path := filepath.Join(t.TempDir(), "broken.jsonl")
		raw := `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":1,"Last":100}` + "\n"
		raw += `{not-json}` + "\n"

		writeErr := os.WriteFile(path, []byte(raw), 0o644)
		rows, skipped, err := LoadMeasurements(path)

		convey.Convey("It should skip malformed lines and continue", func() {
			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Symbol, convey.ShouldEqual, "BTC/EUR")
		})
	})
}

func TestLoadMeasurementsLimit(t *testing.T) {
	convey.Convey("Given a measurement JSONL file with more valid rows than the limit", t, func() {
		path := filepath.Join(t.TempDir(), "measurements.jsonl")
		raw := `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":1,"Last":100}` + "\n"
		raw += `{not-json}` + "\n"
		raw += `{"Symbol":"ETH/EUR","Source":1,"Category":"laminar","SNR":2,"Last":101}` + "\n"
		raw += `{"Symbol":"SOL/EUR","Source":1,"Category":"laminar","SNR":3,"Last":102}` + "\n"

		writeErr := os.WriteFile(path, []byte(raw), 0o644)
		rows, skipped, err := LoadMeasurementsLimit(path, 2)

		convey.Convey("It should stop after the requested valid row count", func() {
			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[1].Symbol, convey.ShouldEqual, "ETH/EUR")
		})
	})

	convey.Convey("Given a negative measurement limit", t, func() {
		rows, skipped, err := LoadMeasurementsLimit("unused.jsonl", -1)

		convey.Convey("It should return an error", func() {
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(rows, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
		})
	})
}

func benchmarkMeasurementJSONL(b *testing.B) string {
	b.Helper()

	path := filepath.Join(b.TempDir(), "measurements.jsonl")
	raw := `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":1,"Last":100}` + "\n"
	raw += `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":101}` + "\n"
	raw += `{"Symbol":"ETH/EUR","Source":12,"Category":"aggressive_drive","SNR":3,"Last":50}` + "\n"

	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		b.Fatal(err)
	}

	return path
}

func BenchmarkLoadMeasurements(b *testing.B) {
	path := benchmarkMeasurementJSONL(b)

	for b.Loop() {
		_, _, err := LoadMeasurements(path)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadMeasurementsLimit(b *testing.B) {
	path := benchmarkMeasurementJSONL(b)

	for b.Loop() {
		_, _, err := LoadMeasurementsLimit(path, 2)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCountMeasurementLines(b *testing.B) {
	path := benchmarkMeasurementJSONL(b)

	for b.Loop() {
		_, _, err := CountMeasurementLines(path)

		if err != nil {
			b.Fatal(err)
		}
	}
}
