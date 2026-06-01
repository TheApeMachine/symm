package optimizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLoadMeasurements(t *testing.T) {
	convey.Convey("Given a measurement JSONL file", t, func() {
		path := filepath.Join(t.TempDir(), "measurements.jsonl")
		raw := []byte(
			`{"Symbol":"BTC/EUR","Source":1,"Category":"laminar","SNR":2,"Last":100}` + "\n" +
				`{"Symbol":"BTC/EUR","Source":12,"Category":"aggressive_drive","SNR":3,"Last":101}` + "\n",
		)

		convey.So(os.WriteFile(path, raw, 0o644), convey.ShouldBeNil)

		rows, err := LoadMeasurements(path)

		convey.Convey("It should decode each measurement row", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(rows[1].Source, convey.ShouldEqual, perspectives.SourceCVD)
		})
	})
}

func TestWriteBranches(t *testing.T) {
	convey.Convey("Given an optimized branch list", t, func() {
		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		branches := perspectives.BranchList{{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1.5,
			ValueSet:  true,
			Action: perspectives.Action{
				Type: perspectives.ActionLimit,
			},
		}}

		err := WriteBranches(path, branches)
		raw, readErr := os.ReadFile(path)

		convey.Convey("It should write a tree document", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "branches:")
			convey.So(string(raw), convey.ShouldContainSubstring, "laminar")
			convey.So(string(raw), convey.ShouldContainSubstring, "value: 1.5")
		})
	})
}
