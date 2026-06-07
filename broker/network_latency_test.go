package broker

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEffectiveNetworkLatencyFromFile(t *testing.T) {
	Convey("Given a latency ring file", t, func() {
		tempDir := t.TempDir()
		linePath := filepath.Join(tempDir, "line_latency.json")
		err := os.WriteFile(linePath, []byte("10000000\n50000000\n20000000\n"), 0o600)
		So(err, ShouldBeNil)

		latency := EffectiveNetworkLatencyFromFile(linePath)

		Convey("It should return the p95 sample from newline integers", func() {
			So(latency, ShouldEqual, 20_000_000)
			So(latency, ShouldNotEqual, 80_000_000)
		})
	})

	Convey("Given a JSON latency sample file", t, func() {
		tempDir := t.TempDir()
		jsonPath := filepath.Join(tempDir, "json_latency.json")
		err := os.WriteFile(jsonPath, []byte(`{"samples":[10000000,50000000,20000000]}`), 0o600)
		So(err, ShouldBeNil)

		latency := EffectiveNetworkLatencyFromFile(jsonPath)

		Convey("It should parse JSON samples", func() {
			So(latency, ShouldEqual, 20_000_000)
		})
	})
}
