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

/*
TestNewImmutableCutKeepsPublishedRowsStable proves a cut freezes the pointer
slice without deep-cloning, and a later Publish upsert replaces thesis rows
without mutating the cut's pointed-to measurements.
*/
func TestNewImmutableCutKeepsPublishedRowsStable(t *testing.T) {
	Convey("Given a published measurement frozen into a cut", t, func() {
		thesis := NewThesis()
		first := time.Unix(1, 0).UTC()
		original := &Measurement{
			Source: SourceHawkes, Metric: MetricEventCount,
			Side: SideBuy, Symbol: "SIM1/USD", Raw: 1, At: first,
		}
		thesis.Publish(SourceHawkes, []*Measurement{original})

		cut := NewImmutableCut(1, 7, thesis)
		So(cut.Measurements, ShouldHaveLength, 1)
		So(cut.Measurements[0].Raw, ShouldEqual, 1)

		Convey("Publish replaces the thesis pointer; cut row stays 1", func() {
			thesis.Publish(SourceHawkes, []*Measurement{{
				Source: SourceHawkes, Metric: MetricEventCount,
				Side: SideBuy, Symbol: "SIM1/USD", Raw: 9, At: time.Unix(2, 0).UTC(),
			}})

			So(thesis.Measurements[0].Raw, ShouldEqual, 9)
			So(cut.Measurements[0].Raw, ShouldEqual, 1)
			So(cut.Measurements[0], ShouldNotEqual, thesis.Measurements[0])
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

func BenchmarkNewImmutableCut(b *testing.B) {
	thesis := NewThesis()
	rows := make([]*Measurement, 0, 512)

	for index := range 512 {
		rows = append(rows, &Measurement{
			Source: SourceHawkes,
			Metric: MetricEventCount,
			Symbol: "SIM1/USD",
			Raw:    float64(index),
			At:     time.Unix(int64(index), 0).UTC(),
		})
	}

	thesis.Publish(SourceHawkes, rows)
	b.ReportAllocs()

	for b.Loop() {
		_ = NewImmutableCut(1, 1, thesis)
	}
}
