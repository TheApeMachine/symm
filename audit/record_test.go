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

		/*
			The stop rows are the only record of what the regulator did between
			two decisions, and the type filter above drops anything it does not
			recognise without erroring. A row that never reaches the file cannot
			be told apart from a stop that never moved.
		*/
		Convey("It should retain stop geometry rows", func() {
			So(Record(recorder, "stop", map[string]any{
				"symbol":      "BTC/USD",
				"reason":      "protected_giveback",
				"hard_floor":  "99.64",
				"profit_line": "100.65",
			}), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeTrue)

			var decoded map[string]any
			So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
			So(decoded["channel"], ShouldEqual, "diagnostic")
			So(decoded["type"], ShouldEqual, "stop")

			value, ok := decoded["value"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(value["reason"], ShouldEqual, "protected_giveback")
			So(value["hard_floor"], ShouldEqual, "99.64")
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
