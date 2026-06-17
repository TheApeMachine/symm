package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func sensoryFixture(base float64) []float64 {
	input := make([]float64, SensoryChannelCount)

	for channelIndex := range input {
		input[channelIndex] = base + float64(channelIndex)*0.01
	}

	return input
}

func TestBatchEngineSettle(testingTB *testing.T) {
	Convey("Given a CPU batch engine", testingTB, func() {
		engine, createErr := newBatchEngine(DefaultArchitecture(), 0.01, 4)

		So(createErr, ShouldBeNil)
		defer engine.Close()

		entries := []batchEntry{
			{slot: 0, symbol: "BTC/USD", input: sensoryFixture(0.1)},
			{slot: 1, symbol: "ETH/USD", input: sensoryFixture(0.2)},
		}

		Convey("It should settle each slot with matching symbols and latent modes", func() {
			outcomes, settleErr := engine.Settle(entries)

			So(settleErr, ShouldBeNil)
			So(len(outcomes), ShouldEqual, len(entries))

			for entryIndex, entry := range entries {
				So(outcomes[entryIndex].symbol, ShouldEqual, entry.symbol)
				So(len(outcomes[entryIndex].latent), ShouldEqual, 3)
				So(outcomes[entryIndex].surprise, ShouldBeGreaterThanOrEqualTo, 0)
				So(outcomes[entryIndex].energy, ShouldBeGreaterThan, 0)
			}
		})

		Convey("It should reject out-of-range slots", func() {
			_, settleErr := engine.Settle([]batchEntry{
				{slot: 99, symbol: "BAD/USD", input: sensoryFixture(0.1)},
			})

			So(settleErr, ShouldNotBeNil)
		})
	})
}

func TestNewBatchEngineInvalidBatchSize(testingTB *testing.T) {
	Convey("Given a non-positive batch size", testingTB, func() {
		_, createErr := newBatchEngine(DefaultArchitecture(), 0.01, 0)

		Convey("It should reject the configuration", func() {
			So(createErr, ShouldNotBeNil)
		})
	})
}

func TestBatchEngineSettleDeterministic(testingTB *testing.T) {
	Convey("Given identical sensory inputs on two engines", testingTB, func() {
		firstEngine, firstErr := newBatchEngine(DefaultArchitecture(), 0.01, 2)
		secondEngine, secondErr := newBatchEngine(DefaultArchitecture(), 0.01, 2)

		So(firstErr, ShouldBeNil)
		So(secondErr, ShouldBeNil)

		defer firstEngine.Close()
		defer secondEngine.Close()

		entry := batchEntry{
			slot:   0,
			symbol: "BTC/USD",
			input:  sensoryFixture(0.15),
		}

		firstOutcomes, firstSettleErr := firstEngine.Settle([]batchEntry{entry})
		secondOutcomes, secondSettleErr := secondEngine.Settle([]batchEntry{entry})

		Convey("It should produce matching surprise on fresh engines", func() {
			So(firstSettleErr, ShouldBeNil)
			So(secondSettleErr, ShouldBeNil)
			So(firstOutcomes[0].surprise, ShouldAlmostEqual, secondOutcomes[0].surprise, 1e-9)
		})
	})
}

func BenchmarkBatchEngineSettleUnit(b *testing.B) {
	engine, err := newBatchEngine(DefaultArchitecture(), 0.01, 8)

	if err != nil {
		b.Fatal(err)
	}

	defer engine.Close()

	entries := make([]batchEntry, 8)

	for slotIndex := range entries {
		entries[slotIndex] = batchEntry{
			slot:   slotIndex,
			symbol: "SYM/USD",
			input:  sensoryFixture(0.1 + float64(slotIndex)*0.01),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, settleErr := engine.Settle(entries)

		if settleErr != nil {
			b.Fatal(settleErr)
		}
	}
}
