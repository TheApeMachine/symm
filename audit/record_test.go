package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRecord(t *testing.T) {
	Convey("Given a diagnostic audit record", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)

		So(err, ShouldBeNil)

		Convey("It should write a diagnostic envelope", func() {
			So(Record(recorder, "decision", map[string]any{
				"symbol": "BTC/USD",
			}), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeTrue)

			var decoded map[string]any
			So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
			So(decoded["channel"], ShouldEqual, "diagnostic")
			So(decoded["type"], ShouldEqual, "decision")
			So(file.Close(), ShouldBeNil)
		})

		Convey("It should drop low-value diagnostic event types", func() {
			So(Record(recorder, "measurement_best_effort", map[string]any{
				"symbol": "BTC/USD",
			}), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeFalse)
			So(file.Close(), ShouldBeNil)
		})

		Convey("It should no-op when the recorder is nil", func() {
			So(Record(nil, "measurement_best_effort", map[string]any{}), ShouldBeNil)
		})
	})
}
