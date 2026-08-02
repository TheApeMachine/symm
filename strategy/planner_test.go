package strategy

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

func TestPlannerUpdate(t *testing.T) {
	Convey("Given a planner thesis gate", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := audit.NewRecorder(path)
		So(err, ShouldBeNil)

		ui := make(chan []byte, 1)
		planner := &Planner{
			ui:          ui,
			recorder:    recorder,
			subscribers: &sync.Map{},
		}
		thesis := types.NewThesis()

		Convey("It should wait until manifold, resonance, causal, and graph are ready", func() {
			returned := planner.Update(thesis)

			So(returned, ShouldEqual, thesis)
			So(len(thesis.Decisions), ShouldEqual, 0)

			var frame map[string]any
			So(json.Unmarshal(<-ui, &frame), ShouldBeNil)
			strategyFrame := frame["strategy"].(map[string]any)
			So(strategyFrame["evaluated"], ShouldBeFalse)
			So(strategyFrame["outcome"], ShouldEqual, "logic_not_ready")

			So(recorder.Close(), ShouldBeNil)
			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()

			scanner := bufio.NewScanner(file)
			phases := make([]string, 0, 2)

			for scanner.Scan() {
				var row map[string]any
				So(json.Unmarshal(scanner.Bytes(), &row), ShouldBeNil)

				value := row["value"].(map[string]any)
				phases = append(phases, value["phase"].(string))
			}

			So(scanner.Err(), ShouldBeNil)
			So(phases, ShouldResemble, []string{"decide_begin", "decide_end"})
		})
	})
}
