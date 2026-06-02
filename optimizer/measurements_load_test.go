package optimizer

import (
	"fmt"
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

func TestLoadMeasurementsSubsampling(t *testing.T) {
	convey.Convey("Given more rows than the sample cap", t, func() {
		path := filepath.Join(t.TempDir(), "large.jsonl")
		lines := make([]byte, 0)

		for index := range 20 {
			line := fmt.Sprintf(
				`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":%d,"Last":100}`+"\n",
				index+1,
			)
			lines = append(lines, []byte(line)...)
		}

		writeErr := os.WriteFile(path, lines, 0o644)
		rows, skipped, err := LoadMeasurements(path, 5)

		convey.Convey("It should subsample evenly spaced rows", func() {
			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 0)
			convey.So(len(rows), convey.ShouldEqual, 5)
		})
	})
}

func TestLoadMeasurementsMalformedLine(t *testing.T) {
	convey.Convey("Given a malformed JSONL line", t, func() {
		path := filepath.Join(t.TempDir(), "broken.jsonl")
		raw := `{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":1,"Last":100}` + "\n"
		raw += `{not-json}` + "\n"

		writeErr := os.WriteFile(path, []byte(raw), 0o644)
		rows, skipped, err := LoadMeasurements(path, 0)

		convey.Convey("It should skip malformed lines and continue", func() {
			convey.So(writeErr, convey.ShouldBeNil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(skipped, convey.ShouldEqual, 1)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Symbol, convey.ShouldEqual, "BTC/EUR")
		})
	})
}
