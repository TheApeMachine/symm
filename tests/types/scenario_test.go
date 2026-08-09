package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScenarioDefaultExecutionConfig(t *testing.T) {
	Convey("Given the default execution contract", t, func() {
		config := DefaultExecutionConfig()

		Convey("It should preserve next-tick top-of-book fills explicitly", func() {
			So(config.DepthLevels, ShouldEqual, 1)
			So(config.DepthQuantityScale, ShouldEqual, 1.0)
			So(config.LimitFillProb, ShouldEqual, 1.0)
			So(config.EnforceBalances, ShouldBeTrue)
			So(config.Validate(), ShouldBeNil)
		})
	})
}

func TestExecutionConfigValidate(t *testing.T) {
	Convey("Given execution probabilities and physical settings", t, func() {
		Convey("A coherent finite-depth partial-fill model should validate", func() {
			config := DefaultExecutionConfig()
			config.DepthLevels = 4
			config.PartialFillProb = 0.5
			config.MeanFragmentCount = 3
			config.FragmentDelay = time.Millisecond

			So(config.Validate(), ShouldBeNil)
		})

		Convey("Contradictory terminal probabilities should be rejected", func() {
			config := DefaultExecutionConfig()
			config.RejectionProb = 0.5
			config.CancellationProb = 0.4
			config.NoFillProb = 0.2

			So(config.Validate(), ShouldNotBeNil)
		})

		Convey("Partial fills without multiple fragments should be rejected", func() {
			config := DefaultExecutionConfig()
			config.PartialFillProb = 1

			So(config.Validate(), ShouldNotBeNil)
		})
	})
}

func TestFaultConfigValidate(t *testing.T) {
	Convey("Given deterministic transport faults", t, func() {
		Convey("Distinct channel occurrences should validate", func() {
			config := FaultConfig{Rules: []FaultRule{
				{Channel: "ticker", Occurrence: 1, Action: FaultDrop},
				{
					Channel: "balances", Occurrence: 2,
					Action: FaultSequenceGap, SequenceGap: 3,
				},
			}}

			So(config.Validate(), ShouldBeNil)
		})

		Convey("Duplicate triggers should be rejected", func() {
			config := FaultConfig{Rules: []FaultRule{
				{Channel: "ticker", Occurrence: 1, Action: FaultDrop},
				{Channel: "ticker", Occurrence: 1, Action: FaultDuplicate},
			}}

			So(config.Validate(), ShouldNotBeNil)
		})
	})
}

func TestScenarioNewScenarioConfig(t *testing.T) {
	Convey("Given explicit symbols", t, func() {
		config := NewScenarioConfig([]*Symbol{
			NewSymbol("BTC/USD", 50_000, 41),
		})

		Convey("It should create a complete replay identity", func() {
			So(config.Name, ShouldNotBeBlank)
			So(config.Seed, ShouldEqual, DefaultScenarioSeed)
			So(config.StartTime, ShouldEqual, DefaultScenarioStart)
			So(config.CandleInterval, ShouldEqual, time.Minute)
			So(config.BookApplyTimeout, ShouldEqual, DefaultBookApplyTimeout)
			So(config.BookPollInterval, ShouldEqual, DefaultBookPollInterval)
			So(config.FlattenTickLimit, ShouldEqual, DefaultFlattenTickLimit)
			So(config.ArtifactDirectory, ShouldEqual, DefaultArtifactDirectory)
			So(config.Validate(), ShouldBeNil)
		})
	})
}

func TestScenarioConfigValidate(t *testing.T) {
	Convey("Given a scheduled multi-symbol scenario", t, func() {
		bitcoin := NewSymbol("BTC/USD", 50_000, 41)
		ethereum := NewSymbol("ETH/USD", 3_000, 42)
		bitcoin.FactorLoading = 0.8
		ethereum.FactorLoading = -0.4
		ethereum.FactorLagTicks = 2
		config := NewScenarioConfig([]*Symbol{bitcoin, ethereum})
		config.Schedule = []RegimeTransition{
			{Tick: 3, Symbol: bitcoin.Pair, State: FastPump},
			{Tick: 5, Symbol: ethereum.Pair, State: FlashCrash},
		}

		Convey("It should validate explicit factor and transition identities", func() {
			So(config.Validate(), ShouldBeNil)
		})

		Convey("An unknown transition symbol should be rejected", func() {
			config.Schedule[0].Symbol = "UNKNOWN/USD"

			So(config.Validate(), ShouldNotBeNil)
		})

		Convey("A missing replay start should be rejected", func() {
			config.StartTime = time.Time{}

			So(config.Validate(), ShouldNotBeNil)
		})

		Convey("A missing regime contract should be rejected", func() {
			config.Profiles = nil

			So(config.Validate(), ShouldNotBeNil)
		})
	})
}
