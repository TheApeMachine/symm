package tables_test

import (
	"cmp"
	"math"
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/tablestest"
)

func writeReplay(t testing.TB, catalog *tables.Catalog, frames []*data.Measurement[float64], omit bool) {
	t.Helper()
	writer := tables.NewWriter(catalog, 1)

	// Even and odd boundaries live in separate, reverse-ordered batches. Replay
	// must interleave them without repeatedly decoding the same binary batch.
	order := make([]int, len(frames))
	for position := range order {
		order[position] = len(frames) - 1 - position
	}
	slices.SortStableFunc(order, func(left, right int) int { return cmp.Compare(left%2, right%2) })
	for position, index := range order {
		frame := frames[index]

		if frame == nil {
			continue
		}

		for peerIndex := len(frame.Peers) - 1; peerIndex >= 0; peerIndex-- {
			if omit && index == 2 && peerIndex == 1 {
				continue
			}

			observation := frame.Peers[peerIndex]
			writer.Add(observation.Provenance["channel"], observation)
		}

		seal := data.NewMeasurement("training", map[string]data.Metric[float64]{
			"previous_input":  {Label: "previous_input", Raw: float64(index)},
			"impulse_version": {Label: "impulse_version", Raw: 1.0},
			"input_count":     {Label: "input_count", Raw: float64(len(frame.Peers))},
		})
		seal.SeqIdx, seal.Label = frame.SeqIdx, frame.Label
		seal.Provenance["owner"] = "training"
		writer.Add("measurements", seal)

		if position == (len(frames)+1)/2-1 {
			if err := writer.CommitReady(t.Context(), true); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := writer.CommitReady(t.Context(), true); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogReplay(t *testing.T) {
	Convey("Binary replay preserves stored floating-point states without JSON substitution", t, func() {
		catalog := tablestest.New(t)
		frames := market.ImpulseTape("BTC/USD", 2)
		values := []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.Copysign(0, -1)}

		for index, value := range values {
			frames[index].Peers[1].Metrics["value"] = data.Metric[float64]{Label: "value", Raw: value}
		}
		writeReplay(t, catalog, frames, false)
		_, recorded, err := catalog.Replay(t.Context(), 1)
		So(err, ShouldBeNil)
		count := 0

		for frame, err := range recorded {
			So(err, ShouldBeNil)

			if err != nil {
				break
			}
			for _, observation := range frame.Peers {
				if observation.Provenance["owner"] == "direct" && count < len(values) {
					So(math.Float64bits(observation.Metrics["value"].Raw), ShouldEqual, math.Float64bits(values[count]))
				}
			}
			count++
		}
		So(count, ShouldEqual, len(frames))
	})

	Convey("Missing persisted producers cannot silently alter historical state", t, func() {
		catalog := tablestest.New(t)
		writeReplay(t, catalog, market.ImpulseTape("BTC/USD", 2), true)
		_, frames, err := catalog.Replay(t.Context(), 1)
		So(err, ShouldBeNil)
		rejected := false

		for _, err := range frames {
			if err != nil {
				rejected = true
				break
			}
		}

		So(rejected, ShouldBeTrue)
	})
}

func TestCatalogReplayMissingBoundary(t *testing.T) {
	Convey("A completely absent boundary is detected by the next input seal", t, func() {
		catalog := tablestest.New(t)
		tape := market.ImpulseTape("BTC/USD", 2)
		tape[2] = nil
		writeReplay(t, catalog, tape, false)
		_, frames, err := catalog.Replay(t.Context(), 1)
		So(err, ShouldBeNil)
		rejected := false

		for _, err := range frames {
			if err != nil {
				So(err.Error(), ShouldContainSubstring, "preceding workspace boundary")
				rejected = true
				break
			}
		}
		So(rejected, ShouldBeTrue)
	})
}

func BenchmarkCatalogReplay(b *testing.B) {
	catalog := tablestest.New(b)
	writeReplay(b, catalog, market.ImpulseTape("BTC/USD", 6), false)
	b.ReportAllocs()
	

	for b.Loop() {
		_, frames, err := catalog.Replay(b.Context(), 1)

		if err != nil {
			b.Fatal(err)
		}

		for _, err := range frames {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
