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
			Measurements: map[string][]*Measurement{
				"BTC/USD": {{
					Symbol: "BTC/USD",
					Source: SourceHawkes,
				}},
			},
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
				So(decoded.Measurements["BTC/USD"][0].Symbol, ShouldEqual, "BTC/USD")
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
		normalized := 0.5
		original := &Measurement{
			Source: SourceHawkes, Symbol: "SIM1/USD", At: first,
			Metrics: map[string]MetricSample{
				MetricKey(MetricEventCount, SideBuy): {
					Raw: 1, Normalized: &normalized,
				},
			},
		}
		thesis.Measurements.Store(original.Key(), original)

		cut := NewImmutableCut(1, 7, thesis)
		So(cut.Measurements, ShouldHaveLength, 1)

		eventCount, ok := cut.Measurements["SIM1/USD"][0].Sample(MetricEventCount, SideBuy)
		So(ok, ShouldBeTrue)
		So(eventCount.Raw, ShouldEqual, 1)

		Convey("Publish replaces the thesis pointer; cut row stays 1", func() {
			replacement := &Measurement{
				Source: SourceHawkes, Symbol: "SIM1/USD", At: time.Unix(2, 0).UTC(),
				Metrics: map[string]MetricSample{
					MetricKey(MetricEventCount, SideBuy): {Raw: 9},
				},
			}
			thesis.Measurements.Store(replacement.Key(), replacement)

			value, _ := thesis.Measurements.Load(replacement.Key())
			liveRow := value.(*Measurement)
			live, ok := liveRow.Sample(MetricEventCount, SideBuy)
			So(ok, ShouldBeTrue)
			So(live.Raw, ShouldEqual, 9)

			frozen, ok := cut.Measurements["SIM1/USD"][0].Sample(MetricEventCount, SideBuy)
			So(ok, ShouldBeTrue)
			So(frozen.Raw, ShouldEqual, 1)
			So(frozen.Normalized, ShouldNotBeNil)
			So(*frozen.Normalized, ShouldEqual, 0.5)
			So(cut.Measurements["SIM1/USD"][0], ShouldNotEqual, liveRow)

			mutated := 9.0
			liveRow.Metrics[MetricKey(MetricEventCount, SideBuy)] = MetricSample{
				Raw: 99, Normalized: &mutated,
			}

			refrozen, ok := cut.Measurements["SIM1/USD"][0].Sample(MetricEventCount, SideBuy)
			So(ok, ShouldBeTrue)
			So(refrozen.Raw, ShouldEqual, 1)
			So(refrozen.Normalized, ShouldNotBeNil)
			So(*refrozen.Normalized, ShouldEqual, 0.5)
		})
	})
}

func BenchmarkImmutableCutCheckpoint(b *testing.B) {
	dir := b.TempDir()
	cut := &ImmutableCut{
		ID:   1,
		Tick: 1,
		At:   time.Now().UTC(),
		Measurements: map[string][]*Measurement{
			"ETH/USD": {{
				Symbol: "ETH/USD",
				Source: SourceDepthFlow,
			}},
		},
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
			Symbol: "SIM1/USD",
			At:     time.Unix(int64(index), 0).UTC(),
			Metrics: map[string]MetricSample{
				MetricKey(MetricEventCount, SideNone): {Raw: float64(index)},
			},
		})
	}

	for _, row := range rows {
		thesis.Measurements.Store(row.Key(), row)
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = NewImmutableCut(1, 1, thesis)
	}
}
