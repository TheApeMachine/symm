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
