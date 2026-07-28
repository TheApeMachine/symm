package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestPhase verifies runtime breadcrumbs carry tick and phase so a freeze can be
located from the last durable row before the next tick.
*/
func TestPhase(t *testing.T) {
	Convey("Given a runtime phase breadcrumb", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)

		Convey("It should record tick-scoped phase rows", func() {
			So(Phase(recorder, 47, "measure_end", map[string]any{
				"signals": 3,
			}), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			t.Cleanup(func() { _ = file.Close() })

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeTrue)

			var decoded map[string]any
			So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
			So(decoded["channel"], ShouldEqual, "diagnostic")
			So(decoded["type"], ShouldEqual, "phase")

			value := decoded["value"].(map[string]any)
			So(value["tick"], ShouldEqual, float64(47))
			So(value["phase"], ShouldEqual, "measure_end")
			So(value["signals"], ShouldEqual, float64(3))
		})

		Convey("It should drop architecture breadcrumbs outside the logic-strategy allowlist", func() {
			So(Phase(recorder, 47, "analyze_begin", map[string]any{
				"signals": 3,
			}), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			t.Cleanup(func() { _ = file.Close() })

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeFalse)
		})

		Convey("It should no-op when the recorder is nil", func() {
			So(Phase(nil, 1, "cut", nil), ShouldBeNil)
		})
	})
}
