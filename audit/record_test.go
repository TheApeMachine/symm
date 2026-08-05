package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestRecord(t *testing.T) {
	Convey("Given a typed analytical event", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)

		Convey("It should write the event under its closed category", func() {
			event := DecisionContext{
				DecisionID: "decision-1",
				Symbol:     "BTC/USD",
				At:         time.Unix(1, 0).UTC(),
				Tick:       7,
				Action:     types.ActionNothing,
				Cause:      "non_positive_utility",
				Reason:     "executable utility did not clear costs",
			}

			So(Record(recorder, event), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeTrue)

			var decoded map[string]any
			So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
			So(decoded["channel"], ShouldEqual, "analysis")
			So(decoded["type"], ShouldEqual, string(CategoryDecisionContext))
		})

		Convey("It should reject invalid evidence instead of persisting defaults", func() {
			event := SignalEvidence{
				DecisionID: "decision-1",
				DecisionAt: time.Unix(2, 0).UTC(),
				Evidence: &types.Measurement{
					Source: types.SourceCVD,
					Symbol: "BTC/USD",
					At:     time.Unix(1, 0).UTC(),
					Validity: types.MeasurementValidity{
						State: types.ValidityProvisional,
					},
					Metrics: map[string]types.MetricSample{
						"delta": {Raw: 0},
					},
				},
			}

			So(Record(recorder, event), ShouldNotBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeFalse)
		})

		Convey("It should no-op when recording is disabled", func() {
			event := ModelValidation{
				Component: "manifold",
				At:        time.Unix(1, 0).UTC(),
				Status:    "filtered",
				Reason:    "invalid_particle",
			}

			So(Record(nil, event), ShouldBeNil)
		})
	})
}
