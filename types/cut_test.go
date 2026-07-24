package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestImmutableCutCheckpoint(t *testing.T) {
	Convey("Given an immutable cut", t, func() {
		dir := t.TempDir()
		cut := &ImmutableCut{
			ID:   3,
			Tick: 9,
			At:   time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
			Measurements: []*Measurement{{
				Symbol: "BTC/USD",
				Source: SourceHawkes,
			}},
		}

		Convey("When Checkpoint persists it", func() {
			So(cut.Checkpoint(dir), ShouldBeNil)

			raw, err := os.ReadFile(filepath.Join(dir, ThesisKey+".json"))
			So(err, ShouldBeNil)

			var decoded ImmutableCut
			So(json.Unmarshal(raw, &decoded), ShouldBeNil)

			Convey("It restores the cut identity and measurements", func() {
				So(decoded.ID, ShouldEqual, CutID(3))
				So(decoded.Tick, ShouldEqual, 9)
				So(decoded.Measurements, ShouldHaveLength, 1)
				So(decoded.Measurements[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func BenchmarkImmutableCutCheckpoint(b *testing.B) {
	dir := b.TempDir()
	cut := &ImmutableCut{
		ID:   1,
		Tick: 1,
		At:   time.Now().UTC(),
		Measurements: []*Measurement{{
			Symbol: "ETH/USD",
			Source: SourceDepthFlow,
		}},
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := cut.Checkpoint(dir); err != nil {
			b.Fatal(err)
		}
	}
}
