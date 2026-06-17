package cognitive

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestBuildSequence(testingTB *testing.T) {
	Convey("Given observations sorted by confidence", testingTB, func() {
		sequence := buildSequence([]Observation{
			{Token: "book_thinning", Confidence: 0.4},
			{Token: "liquidity_shock", Confidence: 0.9},
			{Token: "aggressive_drive", Confidence: 0.7},
		}, "PEAQ/EUR")

		Convey("It should place regime tokens before the symbol suffix", func() {
			So(string(sequence), ShouldEqual, "liquidity_shock_aggressive_drive_book_thinning_PEAQ/EUR")
		})
	})
}

func TestMemorySealScope(testingTB *testing.T) {
	Convey("Given a cognitive memory with trained sensory paths", testingTB, func() {
		memory := NewMemory(context.Background())
		So(memory, ShouldNotBeNil)

		defer memory.Close()

		knownSequence := []byte("liquidity_shock_book_thinning_SOL/EUR")
		unknownSequence := []byte("liquidity_shock_book_thinning_PEAQ/EUR")

		memory.tree.TrainSensorySequence(knownSequence)
		memory.tree.TrainSensorySequence(unknownSequence)

		_, _ = memory.tree.InsertAttractorBasin(
			[]byte("liquidity_vacuum"),
			[]byte("liquidity_shock_book_thinning"),
			dmt.CognitiveState{Count: 8, Probability: 0.82},
		)

		memory.observe("PEAQ/EUR", Observation{
			Token:      "liquidity_shock",
			Confidence: 0.91,
		})
		memory.observe("PEAQ/EUR", Observation{
			Token:      "book_thinning",
			Confidence: 0.88,
		})

		reading := memory.SealScope("PEAQ/EUR", time.Unix(0, 1))

		Convey("It should seal a regime-first sequence and expose a reading", func() {
			So(reading, ShouldNotBeNil)
			So(string(reading.Sequence), ShouldEqual, string(unknownSequence))
			So(string(reading.RegimePrefix), ShouldEqual, "liquidity_shock_book_thinning")
			So(reading.ClassConfidence, ShouldBeGreaterThan, 0)
			So(reading.RegimeCohort, ShouldContain, "SOL/EUR")
			So(reading.RegimeCohort, ShouldContain, "PEAQ/EUR")
		})

		Convey("It should resolve structural analog profiles across symbols", func() {
			profile := []byte(`{"slippage_bps":12,"size_fraction":0.5}`)
			_, stored := memory.StoreProfile(knownSequence, profile)

			So(stored, ShouldBeTrue)

			value, found := memory.LookupProfile(unknownSequence)

			So(found, ShouldBeTrue)
			So(string(value), ShouldEqual, string(profile))
		})
	})
}

func TestApplyAction(testingTB *testing.T) {
	Convey("Given a reading with high contrast evidence", testingTB, func() {
		memory := NewMemory(context.Background())
		So(memory, ShouldNotBeNil)

		defer memory.Close()

		reading := &Reading{
			Sequence:         []byte("unique_contrast_test_XRP/EUR"),
			ClassConfidence:  0.8,
			ContrastEvidence: 0.9,
		}

		action := logic.NewAction(
			logic.ActionMarket,
			trading.Buy,
			"BTC/EUR",
			100,
			1,
			0,
			0.5,
			"",
		)
		action.EntryConfidence = 0.4

		memory.ApplyAction(action, reading)

		Convey("It should scale fraction and confidence from contrast evidence", func() {
			So(action.Fraction, ShouldBeGreaterThan, 0.5)
			So(action.EntryConfidence, ShouldBeGreaterThan, 0.4)
		})
	})
}

func TestRecordOutcome(testingTB *testing.T) {
	Convey("Given a sealed sequence", testingTB, func() {
		memory := NewMemory(context.Background())
		So(memory, ShouldNotBeNil)

		defer memory.Close()

		sequence := []byte("liquidity_shock_BTC/EUR")
		profile := ProfileFromExecution(8, 0.75)

		memory.RecordOutcome(sequence, profile, time.Unix(0, 42).UnixNano())

		Convey("It should store profiles and commit episodic memory", func() {
			stored, found := memory.LookupProfile(sequence)

			So(found, ShouldBeTrue)
			So(string(stored), ShouldContainSubstring, "slippage_bps")
		})
	})
}

func BenchmarkMemorySealScope(b *testing.B) {
	memory := NewMemory(context.Background())

	if memory == nil {
		b.Fatal("NewMemory returned nil")
	}

	defer memory.Close()

	observations := []Observation{
		{Token: "liquidity_shock", Confidence: 0.91},
		{Token: "book_thinning", Confidence: 0.88},
		{Token: "aggressive_drive", Confidence: 0.75},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, observation := range observations {
			memory.observe("SOL/EUR", observation)
		}

		reading := memory.SealScope("SOL/EUR", time.Now())

		if reading == nil {
			b.Fatal("SealScope returned nil")
		}
	}
}
