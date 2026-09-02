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
as-of resolution can be exercised against an exactly known history. The map key
is sequence+ordinal so multi-envelope raw captures can be modelled.
*/
type tapeReader struct {
	states map[uint64][]byte
	reads  []uint64
}

func tapeKey(sequence, ordinal uint64) uint64 {
	return sequence*10 + ordinal
}

func (reader *tapeReader) ReadStatePayload(
	_ string,
	sequence uint64,
	ordinal uint64,
) ([]byte, bool, error) {
	reader.reads = append(reader.reads, sequence)
	payload, present := reader.states[tapeKey(sequence, ordinal)]

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

/*
stateWithPerspectives encodes one historical EnvelopeState carrying the given
retired Perspective frames. Each frame declares its own Kind, so legacy
advisor-family rows can be separated by identity.
*/
func stateWithPerspectives(frames []*wire.PerspectiveFrameT) []byte {
	state := &wire.EnvelopeStateT{Perspectives: frames}

	builder := flatbuffers.NewBuilder(1024)
	builder.Finish(state.Pack(builder))

	return builder.FinishedBytes()
}

func reading(metric string, value float64, defined bool) *wire.PerspectiveReadingT {
	return &wire.PerspectiveReadingT{
		Metric:     metric,
		Value:      value,
		Defined:    defined,
		ObservedAt: 1000,
		From:       500,
		Maturity:   0.7,
		Snr:        2.5,
		SnrDefined: true,
	}
}

func refCoordinates(coordinates ...[2]uint64) []EnvelopeRef {
	refs := make([]EnvelopeRef, 0, len(coordinates))

	for _, coordinate := range coordinates {
		sequence := coordinate[0]
		ordinal := coordinate[1]
		refs = append(refs, EnvelopeRef{
			Origin: CaptureIdentity{
				Run:            "run-test",
				Sequence:       CaptureSequence(sequence),
				Stream:         "spot.public",
				StreamEpoch:    1,
				StreamSequence: sequence,
			},
			Ordinal: ordinal,
		})
	}

	return refs
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

func targetRef(sequence CaptureSequence, ordinal uint64) EnvelopeRef {
	return EnvelopeRef{
		Origin: CaptureIdentity{
			Run:            "run-test",
			Sequence:       sequence,
			Stream:         "spot.public",
			StreamEpoch:    1,
			StreamSequence: uint64(sequence),
		},
		Ordinal: ordinal,
	}
}

func TestResolveResidentTest(t *testing.T) {
	Convey("Given a tape where each envelope carried different signals", t, func() {
		base := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
		mark := base.Add(10 * time.Second)

		reader := &tapeReader{states: map[uint64][]byte{
			tapeKey(10, 0): stateWith(10, base.UnixNano(), []string{"hawkes"}, "vertical_ignition", 0.91),
			tapeKey(20, 0): stateWith(20, base.Add(4*time.Second).UnixNano(), []string{"cvd"}, "", 0),
			tapeKey(30, 0): stateWith(30, base.Add(9*time.Second).UnixNano(), []string{"liquidity"}, "", 0),
			// After the mark: causally unavailable, and never to be consulted.
			tapeKey(40, 0): stateWith(40, base.Add(20*time.Second).UnixNano(), []string{"pumpDump"}, "", 0),
		}}

		resident, err := ResolveResident(
			"run-test",
			"TEST/USD",
			targetRef(30, 0),
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
			tapeKey(10, 0): stateWith(10, base.UnixNano(), []string{"hawkes"}, "", 0),
			tapeKey(20, 0): stateWith(20, base.Add(9*time.Second).UnixNano(), []string{"cvd"}, "", 0),
		}

		before, err := ResolveResident(
			"run-test", "TEST/USD", targetRef(20, 0), mark,
			refsDescending(20, 10), &tapeReader{states: tape}, 64,
		)
		So(err, ShouldBeNil)

		Convey("Rewriting every later observation changes nothing at the coordinate", func() {
			tape[tapeKey(30, 0)] = stateWith(30, base.Add(30*time.Second).UnixNano(),
				[]string{"hawkes", "cvd", "liquidity", "pumpDump"}, "late_category", 1)
			tape[tapeKey(40, 0)] = stateWith(40, base.Add(40*time.Second).UnixNano(),
				[]string{"hawkes", "cvd"}, "", 0)

			after, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(20, 0), mark,
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
			tapeKey(1, 0):  stateWith(1, base.UnixNano(), []string{"hawkes"}, "", 0),
			tapeKey(98, 0): stateWith(98, base.Add(time.Second).UnixNano(), []string{"cvd"}, "", 0),
			tapeKey(99, 0): stateWith(99, base.Add(2*time.Second).UnixNano(), []string{"liquidity"}, "", 0),
		}

		resident, err := ResolveResident(
			"run-test", "TEST/USD", targetRef(99, 0), base.Add(2*time.Second),
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

/*
TestResolveResidentMultiOrdinalTests is §D: one raw capture produces many
envelopes; causal availability is lexicographic over (CaptureSequence,
Ordinal), and the futures inside the same capture never leak backwards.
*/
func TestResolveResidentMultiOrdinalTest(t *testing.T) {
	Convey("Given one raw capture with three envelopes", t, func() {
		base := time.Date(2026, 8, 30, 21, 0, 0, 0, time.UTC)
		mark := base.Add(time.Second)

		reader := &tapeReader{states: map[uint64][]byte{
			tapeKey(99, 0):  stateWith(99, base.UnixNano(), []string{"hawkes"}, "", 0),
			tapeKey(100, 0): stateWith(100, base.Add(100*time.Millisecond).UnixNano(), []string{"cvd"}, "", 0),
			tapeKey(100, 1): stateWith(100, base.Add(200*time.Millisecond).UnixNano(), []string{"liquidity"}, "", 0),
			tapeKey(100, 2): stateWith(100, base.Add(300*time.Millisecond).UnixNano(), []string{"pumpDump"}, "", 0),
		}}

		candidates := refCoordinates(
			[2]uint64{100, 2},
			[2]uint64{100, 1},
			[2]uint64{100, 0},
			[2]uint64{99, 0},
		)

		Convey("Target 100/0 cannot see 100/1 or 100/2", func() {
			resident, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(100, 0), mark,
				candidates, reader, 64,
			)

			So(err, ShouldBeNil)

			bySource := make(map[string]ResidentMeasurement)

			for _, signal := range resident.Signals {
				bySource[signal.Source] = signal
			}

			So(bySource["cvd"].Carried, ShouldBeFalse)
			So(bySource["hawkes"].Carried, ShouldBeTrue)

			_, future := bySource["liquidity"]
			So(future, ShouldBeFalse)
			_, future = bySource["pumpDump"]
			So(future, ShouldBeFalse)
		})

		Convey("Target 100/2 sees 100/1 but not 100/3", func() {
			resident, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(100, 2), mark,
				candidates, reader, 64,
			)

			So(err, ShouldBeNil)

			bySource := make(map[string]ResidentMeasurement)

			for _, signal := range resident.Signals {
				bySource[signal.Source] = signal
			}

			So(bySource["pumpDump"].Carried, ShouldBeFalse)
			So(bySource["liquidity"].Carried, ShouldBeTrue)
			So(bySource["cvd"].Carried, ShouldBeTrue)
			So(bySource["hawkes"].Carried, ShouldBeTrue)
		})

		Convey("A value originating exactly at the target is fresh, not carried", func() {
			resident, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(100, 2), mark,
				candidates, reader, 64,
			)

			So(err, ShouldBeNil)

			for _, signal := range resident.Signals {
				if signal.Source == "pumpDump" {
					So(signal.Carried, ShouldBeFalse)
				}
			}
		})

		Convey("An earlier ordinal in the same capture is carried", func() {
			resident, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(100, 2), mark,
				candidates, reader, 64,
			)

			So(err, ShouldBeNil)

			for _, signal := range resident.Signals {
				if signal.Source == "liquidity" {
					So(signal.Carried, ShouldBeTrue)
					So(signal.Origin.Origin.Sequence, ShouldEqual, CaptureSequence(100))
					So(signal.Origin.Ordinal, ShouldEqual, 1)
				}
			}
		})

		Convey("An earlier capture sequence is carried", func() {
			resident, err := ResolveResident(
				"run-test", "TEST/USD", targetRef(100, 0), mark,
				candidates, reader, 64,
			)

			So(err, ShouldBeNil)

			for _, signal := range resident.Signals {
				if signal.Source == "hawkes" {
					So(signal.Carried, ShouldBeTrue)
					So(signal.Origin.Origin.Sequence, ShouldEqual, CaptureSequence(99))
				}
			}
		})
	})
}

/*
TestResolveResidentKindIdentityTest proves multiple retired advisor families
for the same symbol+peer all survive historical reconstruction: Kind is part of
the legacy identity, so they never collapse on the old symbol|peer key.
*/
func TestResolveResidentKindIdentityTest(t *testing.T) {
	Convey("Given one historical capture carrying several legacy kinds", t, func() {
		mark := time.Date(2026, 8, 30, 22, 0, 0, 0, time.UTC)

		frames := []*wire.PerspectiveFrameT{
			{Symbol: "POND/USD", Peer: "", Kind: 1, At: mark.UnixNano(),
				Readings: []*wire.PerspectiveReadingT{reading("liquidity", 1, true)}},
			{Symbol: "POND/USD", Peer: "", Kind: 2, At: mark.UnixNano(),
				Readings: []*wire.PerspectiveReadingT{reading("depth", 2, true)}},
			{Symbol: "POND/USD", Peer: "", Kind: 3, At: mark.UnixNano(),
				Readings: []*wire.PerspectiveReadingT{reading("spread", 3, true)}},
		}

		reader := &tapeReader{states: map[uint64][]byte{
			tapeKey(100, 0): stateWithPerspectives(frames),
		}}

		resident, err := ResolveResident(
			"run-test", "POND/USD", targetRef(100, 0), mark,
			refCoordinates([2]uint64{100, 0}), reader, 64,
		)

		So(err, ShouldBeNil)

		Convey("All three kinds survive as distinct resident rows", func() {
			So(len(resident.Views), ShouldEqual, 3)

			kinds := make([]uint8, 0, 3)

			for _, view := range resident.Views {
				kinds = append(kinds, view.Kind)
			}

			So(kinds, ShouldContain, uint8(1))
			So(kinds, ShouldContain, uint8(2))
			So(kinds, ShouldContain, uint8(3))
		})

		Convey("Per-reading provenance round-trips", func() {
			for _, view := range resident.Views {
				So(len(view.Readings), ShouldEqual, 1)

				residentReading := view.Readings[0]
				So(residentReading.ObservedAt, ShouldEqual, 1000)
				So(residentReading.HasAt, ShouldBeTrue)
				So(residentReading.From, ShouldEqual, 500)
				So(residentReading.HasFrom, ShouldBeTrue)
				So(residentReading.Maturity, ShouldAlmostEqual, 0.7)
				So(residentReading.SNR, ShouldAlmostEqual, 2.5)
				So(residentReading.SNRDefined, ShouldBeTrue)
			}
		})
	})
}
