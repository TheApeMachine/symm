package hindsight

import (
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/smartystreets/goconvey/convey"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
tapeReader is a StateReader over an in-memory tape of encoded states, so the
as-of resolution can be exercised against an exactly known history.
*/
type tapeReader struct {
	states map[uint64][]byte
	reads  []uint64
}

func (reader *tapeReader) ReadStatePayload(
	_ string,
	sequence uint64,
	_ uint64,
) ([]byte, bool, error) {
	reader.reads = append(reader.reads, sequence)
	payload, present := reader.states[sequence]

	return payload, present, nil
}

/*
stateWith encodes one historical EnvelopeState carrying exactly the named
signal families, plus an optional category, at a declared instant.
*/
func stateWith(sequence uint64, atNs int64, sources []string, category string, confidence float64) []byte {
	state := &wire.EnvelopeStateT{CaptureSeq: sequence}

	for _, source := range sources {
		measurement := &wire.EnvelopeMeasurementT{
			Id:       source + ":TEST/USD",
			Source:   source,
			AtNs:     atNs,
			Maturity: 0.5,
			Metrics: []*wire.EnvelopeMeasurementMetricT{{
				Key: "value",
				Value: &wire.EnvelopeMetricT{
					Label: "value",
					Raw:   float64(sequence),
					Unit:  "dimensionless",
				},
			}},
		}

		switch source {
		case "cvd":
			state.Cvd = measurement
		case "hawkes":
			state.Hawkes = measurement
		case "liquidity":
			state.Liquidity = measurement
		case "pumpDump":
			state.PumpDump = measurement
		}
	}

	if category != "" {
		state.Categories = []*wire.EnvelopeCategoryT{{
			Type:       category,
			AtNs:       atNs,
			Confidence: confidence,
		}}
	}

	builder := flatbuffers.NewBuilder(1024)
	builder.Finish(state.Pack(builder))

	return builder.FinishedBytes()
}

func refsDescending(sequences ...uint64) []EnvelopeRef {
	refs := make([]EnvelopeRef, 0, len(sequences))

	for _, sequence := range sequences {
		refs = append(refs, EnvelopeRef{
			Origin: CaptureIdentity{
				Run:            "run-test",
				Sequence:       CaptureSequence(sequence),
				Stream:         "spot.public",
				StreamEpoch:    1,
				StreamSequence: sequence,
			},
		})
	}

	return refs
}

func TestResolveResidentTest(t *testing.T) {
	Convey("Given a tape where each envelope carried different signals", t, func() {
		base := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
		mark := base.Add(10 * time.Second)

		reader := &tapeReader{states: map[uint64][]byte{
			10: stateWith(10, base.UnixNano(), []string{"hawkes"}, "vertical_ignition", 0.91),
			20: stateWith(20, base.Add(4*time.Second).UnixNano(), []string{"cvd"}, "", 0),
			30: stateWith(30, base.Add(9*time.Second).UnixNano(), []string{"liquidity"}, "", 0),
			// After the mark: causally unavailable, and never to be consulted.
			40: stateWith(40, base.Add(20*time.Second).UnixNano(), []string{"pumpDump"}, "", 0),
		}}

		resident, err := ResolveResident(
			"run-test",
			"TEST/USD",
			CaptureSequence(30),
			mark,
			refsDescending(40, 30, 20, 10),
			reader,
			64,
		)

		So(err, ShouldBeNil)

		bySource := make(map[string]ResidentMeasurement)

		for _, signal := range resident.Signals {
			bySource[signal.Source] = signal
		}

		Convey("Each family resolves to the latest value at or before the mark", func() {
			So(bySource["liquidity"].Origin.Origin.Sequence, ShouldEqual, CaptureSequence(30))
			So(bySource["cvd"].Origin.Origin.Sequence, ShouldEqual, CaptureSequence(20))
			So(bySource["hawkes"].Origin.Origin.Sequence, ShouldEqual, CaptureSequence(10))
		})

		Convey("A value from an earlier envelope is reported as carried, with its age", func() {
			So(bySource["liquidity"].Carried, ShouldBeFalse)
			So(bySource["cvd"].Carried, ShouldBeTrue)
			So(bySource["hawkes"].Carried, ShouldBeTrue)
			So(bySource["hawkes"].AgeNs, ShouldEqual, int64(10*time.Second))
			So(bySource["cvd"].AgeNs, ShouldEqual, int64(6*time.Second))
			So(bySource["liquidity"].AgeNs, ShouldEqual, int64(time.Second))
		})

		Convey("A family never produced at or before the mark is unresolved, not zero", func() {
			So(resident.Unresolved, ShouldContain, "pumpDump")
			_, present := bySource["pumpDump"]
			So(present, ShouldBeFalse)
		})

		Convey("The future is not read at all", func() {
			So(reader.reads, ShouldNotContain, uint64(40))
		})

		Convey("A category resolves the same way and reports its own age", func() {
			So(len(resident.Categories), ShouldEqual, 1)
			So(resident.Categories[0].Type, ShouldEqual, "vertical_ignition")
			So(resident.Categories[0].Confidence, ShouldAlmostEqual, 0.91)
			So(resident.Categories[0].Carried, ShouldBeTrue)
		})

		Convey("The resolved values are the ones those envelopes actually carried", func() {
			So(bySource["hawkes"].Metrics[0].Raw, ShouldEqual, 10.0)
			So(bySource["cvd"].Metrics[0].Raw, ShouldEqual, 20.0)
			So(bySource["liquidity"].Metrics[0].Raw, ShouldEqual, 30.0)
		})
	})
}

/*
TestResolveResidentFutureLeakTest is §65.9 applied to as-of resolution: the
future may select the coordinate, but changing everything after it must not
change one value resolved there.
*/
func TestResolveResidentFutureLeakTest(t *testing.T) {
	Convey("Given a resolved coordinate", t, func() {
		base := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
		mark := base.Add(10 * time.Second)

		tape := map[uint64][]byte{
			10: stateWith(10, base.UnixNano(), []string{"hawkes"}, "", 0),
			20: stateWith(20, base.Add(9*time.Second).UnixNano(), []string{"cvd"}, "", 0),
		}

		before, err := ResolveResident(
			"run-test", "TEST/USD", CaptureSequence(20), mark,
			refsDescending(20, 10), &tapeReader{states: tape}, 64,
		)
		So(err, ShouldBeNil)

		Convey("Rewriting every later observation changes nothing at the coordinate", func() {
			tape[30] = stateWith(30, base.Add(30*time.Second).UnixNano(),
				[]string{"hawkes", "cvd", "liquidity", "pumpDump"}, "late_category", 1)
			tape[40] = stateWith(40, base.Add(40*time.Second).UnixNano(),
				[]string{"hawkes", "cvd"}, "", 0)

			after, err := ResolveResident(
				"run-test", "TEST/USD", CaptureSequence(20), mark,
				refsDescending(40, 30, 20, 10), &tapeReader{states: tape}, 64,
			)

			So(err, ShouldBeNil)
			So(len(after.Signals), ShouldEqual, len(before.Signals))

			for index := range after.Signals {
				So(after.Signals[index].Source, ShouldEqual, before.Signals[index].Source)
				So(
					after.Signals[index].Origin.Origin.Sequence,
					ShouldEqual,
					before.Signals[index].Origin.Origin.Sequence,
				)
				So(
					after.Signals[index].Metrics[0].Raw,
					ShouldEqual,
					before.Signals[index].Metrics[0].Raw,
				)
			}

			So(after.Categories, ShouldBeEmpty)
		})
	})
}

func TestResolveResidentBudgetTest(t *testing.T) {
	Convey("Given a search budget shorter than the history it would need", t, func() {
		base := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
		tape := map[uint64][]byte{
			1:  stateWith(1, base.UnixNano(), []string{"hawkes"}, "", 0),
			98: stateWith(98, base.Add(time.Second).UnixNano(), []string{"cvd"}, "", 0),
			99: stateWith(99, base.Add(2*time.Second).UnixNano(), []string{"liquidity"}, "", 0),
		}

		resident, err := ResolveResident(
			"run-test", "TEST/USD", CaptureSequence(99), base.Add(2*time.Second),
			refsDescending(99, 98, 1), &tapeReader{states: tape}, 2,
		)

		So(err, ShouldBeNil)

		Convey("The walk stops at the budget and says so, rather than claiming absence", func() {
			So(resident.Examined, ShouldEqual, 2)
			So(resident.Exhausted, ShouldBeTrue)
			So(resident.Unresolved, ShouldContain, "hawkes")
		})
	})
}
