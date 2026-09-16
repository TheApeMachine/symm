package tables_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/strategy/impulse"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/tablestest"
)

func writeReplay(t testing.TB, catalog *tables.Catalog, frames []*data.Measurement[float64], omit bool) {
	t.Helper()
	writer := tables.NewWriter(catalog, 1)

	// Two batches in reverse event order prove that file/append order is irrelevant.
	for index := len(frames) - 1; index >= 0; index-- {
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

		seal := data.NewMeasurement[float64]("training", map[string]data.Metric[float64]{
			"previous_input":  {Label: "previous_input", Raw: float64(index)},
			"impulse_version": {Label: "impulse_version", Raw: grid.FormatVersion},
			"input_count":     {Label: "input_count", Raw: float64(len(frame.Peers))},
		})
		seal.SeqIdx, seal.Label = frame.SeqIdx, frame.Label
		seal.Provenance["owner"] = "training"
		writer.Add("measurements", seal)

		if index == len(frames)/2 {
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
	Convey("Recorded owner values reconstruct the same coordinates and regions", t, func() {
		catalog := tablestest.New(t)
		frames := market.ImpulseTape("BTC/USD", 6)
		frames[2].Peers[1].Err = errors.New("fixture: rejected quote")
		writeReplay(t, catalog, frames, false)
		live, replay := impulse.NewMap(), impulse.NewMap()
		expected := make(map[int64]any)

		for _, frame := range frames {
			So(live.Step(frame), ShouldBeNil)
			expected[frame.SeqIdx] = live.Markets["BTC/USD"].Snapshot()
		}

		_, recorded, err := catalog.Replay(t.Context(), 1)
		So(err, ShouldBeNil)
		count := 0

		for frame, err := range recorded {
			So(err, ShouldBeNil)

			if err != nil {
				break
			}

			// Venue times and iteration order cannot affect the calculation.
			slices.Reverse(frame.Peers)

			for _, observation := range frame.Peers {
				observation.At = time.Time{}
			}

			So(replay.Step(frame), ShouldBeNil)
			So(replay.Markets["BTC/USD"].Snapshot(), ShouldResemble, expected[frame.SeqIdx])
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
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
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
