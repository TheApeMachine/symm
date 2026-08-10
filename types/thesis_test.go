package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestThesisAppendMeasurements(t *testing.T) {
	Convey("Given an empty measurement pass", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)

		thesis.AppendMeasurements(SourceCorrelation, nil, false)

		Convey("Then it should not create an indirect notification cycle", func() {
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
		})
	})

	Convey("Given an empty pass that claims readiness", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)
		thesis.Subscribe(SourceCategories, categories)

		thesis.AppendMeasurements(SourceCorrelation, nil, true)

		Convey("Then it should stamp readiness and notify the downstream stage", func() {
			So(thesis.Stamped(SourceCorrelation), ShouldBeTrue)
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
			So(len(categories), ShouldEqual, 1)
		})
	})

	Convey("Given multiple measurements from one signal", t, func() {
		thesis := NewThesis(t.Context(), nil)
		measurements := []*Measurement{
			{ID: "btc", Source: SourceLeadLag, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{ID: "eth", Source: SourceLeadLag, Symbol: "ETH/USD", At: time.Unix(2, 0)},
			{ID: "sol", Source: SourceLeadLag, Symbol: "SOL/USD", At: time.Unix(3, 0)},
		}

		thesis.AppendMeasurements(SourceLeadLag, measurements, true)

		Convey("Then it should retain each measurement exactly once in source order", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			actual := stored.([]*Measurement)
			So(actual, ShouldResemble, measurements)
			So(thesis.Readiness.LeadLag, ShouldBeTrue)
		})
	})

	Convey("Given a new measurement for one previously observed symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := &Measurement{
			ID:     "bitcoin-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}
		ether := &Measurement{
			ID:     "ether-1",
			Source: SourceHawkes,
			Symbol: "ETH/USD",
			At:     time.Unix(1, 0),
		}
		updated := &Measurement{
			ID:     "bitcoin-2",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
		}
		thesis.AppendMeasurements(SourceHawkes, []*Measurement{bitcoin, ether}, false)
		thesis.AppendMeasurements(SourceHawkes, []*Measurement{updated}, false)

		Convey("Then it should replace that identity and retain the other symbol", func() {
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{updated, ether})
		})
	})

	Convey("Given multiple peer measurements sharing one symbol", t, func() {
		thesis := NewThesis(t.Context(), nil)
		ether := &Measurement{
			ID: "btc-eth-1", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "ETH/USD", At: time.Unix(1, 0),
		}
		solana := &Measurement{
			ID: "btc-sol-1", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "SOL/USD", At: time.Unix(1, 0),
		}
		updated := &Measurement{
			ID: "btc-eth-2", Source: SourceLeadLag,
			Symbol: "BTC/USD", Peer: "ETH/USD", At: time.Unix(2, 0),
		}
		thesis.AppendMeasurements(SourceLeadLag, []*Measurement{ether, solana}, false)
		thesis.AppendMeasurements(SourceLeadLag, []*Measurement{updated}, false)

		Convey("Then it should replace only the matching peer", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{updated, solana})
		})
	})

	Convey("Given a measurement epoch whose market input is already committed", t, func() {
		thesis := NewThesis(t.Context(), nil)
		at := time.Unix(1, 0).UTC()
		trade := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: at,
		}
		measurement := &Measurement{
			ID:     "cvd-1",
			Source: SourceCVD,
			Symbol: trade.Symbol,
			At:     at,
		}

		thesis.AppendTrade(trade)
		So(thesis.MarketTrades(SourceCVD), ShouldHaveLength, 1)
		So(thesis.AppendMeasurements(
			SourceCVD, []*Measurement{measurement}, false,
		), ShouldBeNil)

		Convey("Then replaying that measurement should return a conflict", func() {
			err := thesis.AppendMeasurements(
				SourceCVD, []*Measurement{measurement}, false,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"thesis: duplicate measurement found for [cvd]")
			stored, found := thesis.Measurements.Load(SourceCVD)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{measurement})
		})
	})

	Convey("Given distinct measurements sharing one timestamp", t, func() {
		thesis := NewThesis(t.Context(), nil)
		at := time.Unix(1, 0).UTC()
		first := &Measurement{
			ID:     "hawkes-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     at,
		}
		updated := &Measurement{
			ID:       "hawkes-2",
			Source:   SourceHawkes,
			Symbol:   "BTC/USD",
			At:       at,
			Maturity: 1,
		}

		So(thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{first}, false,
		), ShouldBeNil)

		Convey("Then the new ID should replace the prior measurement", func() {
			err := thesis.AppendMeasurements(
				SourceHawkes, []*Measurement{updated}, false,
			)

			So(err, ShouldBeNil)
			stored, found := thesis.Measurements.Load(SourceHawkes)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, []*Measurement{updated})
		})
	})

	Convey("Given a reader holding the prior measurement slice", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := &Measurement{
			ID:     "bitcoin-1",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}
		ether := &Measurement{
			ID:     "ether-1",
			Source: SourceHawkes,
			Symbol: "ETH/USD",
			At:     time.Unix(1, 0),
		}
		updated := &Measurement{
			ID:     "bitcoin-2",
			Source: SourceHawkes,
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
		}
		thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{bitcoin, ether}, false,
		)
		stored, _ := thesis.Measurements.Load(SourceHawkes)
		prior := stored.([]*Measurement)
		thesis.AppendMeasurements(
			SourceHawkes, []*Measurement{updated}, false,
		)

		Convey("Then every pointer in the prior view should remain valid", func() {
			for _, measurement := range prior {
				So(measurement, ShouldNotBeNil)
			}
		})
	})
}

func TestThesisFanout(t *testing.T) {
	Convey("Given signal and downstream subscribers", t, func() {
		thesis := NewThesis(t.Context(), nil)
		correlation := make(chan struct{}, 1)
		cvd := make(chan struct{}, 1)
		categories := make(chan struct{}, 1)
		thesis.Subscribe(SourceCorrelation, correlation)
		thesis.Subscribe(SourceCVD, cvd)
		thesis.Subscribe(SourceCategories, categories)

		thesis.Fanout(SourceCorrelation)

		Convey("Then signal output should only wake downstream work", func() {
			So(len(correlation), ShouldEqual, 0)
			So(len(cvd), ShouldEqual, 0)
			So(len(categories), ShouldEqual, 1)
		})

		Convey("Then market input should wake every signal", func() {
			thesis.Fanout(SourceTrader)

			So(len(correlation), ShouldEqual, 1)
			So(len(cvd), ShouldEqual, 1)
		})
	})
}

func TestThesisObserveCompletions(t *testing.T) {
	Convey("Given a completed Thesis epoch", t, func() {
		thesis := NewThesis(t.Context(), nil)
		observations := make([]EpochObservation, 0, 1)
		thesis.Tick = 42
		thesis.ObserveCompletions(func(observation EpochObservation) {
			observations = append(observations, observation)
		})

		for _, source := range []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceToxicity,
			SourceCategories,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			thesis.Stamp(source)
		}

		thesis.Fanout(SourcePlanner)
		thesis.Fanout(SourcePlanner)
		thesis.Reset()

		Convey("Then one detached pre-reset identity should be observed", func() {
			So(observations, ShouldHaveLength, 1)
			So(observations[0].Tick, ShouldEqual, 42)
			So(observations[0].At.IsZero(), ShouldBeFalse)
			So(observations[0].At.Equal(thesis.At), ShouldBeFalse)
		})
	})
}

func TestThesisReset(t *testing.T) {
	Convey("Given a completed Thesis with ready measurement evidence", t, func() {
		thesis := NewThesis(t.Context(), nil)
		notified := make(chan struct{}, 1)
		thesis.Subscribe(SourceCategories, notified)
		measurements := []*Measurement{{
			Source: SourceToxicity,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
		}}
		consumed := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 1, Timestamp: time.Unix(1, 0),
		}
		thesis.AppendTrade(consumed)
		So(thesis.MarketTrades(SourceToxicity), ShouldHaveLength, 1)
		thesis.AppendMeasurements(SourceToxicity, measurements, true)
		thesis.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})

		for _, source := range []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceToxicity,
			SourceCategories,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			thesis.Stamp(source)
		}

		So(thesis.Readiness.Complete(), ShouldBeTrue)
		<-notified
		thesis.Reset()

		Convey("Then the next epoch should retain only the prior measurements", func() {
			stored, found := thesis.Measurements.Load(SourceToxicity)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, measurements)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
			So(len(notified), ShouldEqual, 0)
			_, found = thesis.Categories.Load("BTC/USD")
			So(found, ShouldBeFalse)

			thesis.AppendTrade(consumed)
			thesis.AppendTrade(kraken.TradeData{
				Symbol: "BTC/USD", TradeID: 2, Timestamp: time.Unix(2, 0),
			})
			unseen := thesis.MarketTrades(SourceToxicity)
			So(unseen, ShouldHaveLength, 1)
			So(unseen[0].TradeID, ShouldEqual, 2)
		})
	})
}

func BenchmarkThesisAppendMeasurements(b *testing.B) {
	b.Run("empty pass", func(b *testing.B) {
		thesis := NewThesis(b.Context(), nil)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			if err := thesis.AppendMeasurements(SourceCorrelation, nil, false); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("replacement", func(b *testing.B) {
		thesis := NewThesis(b.Context(), nil)
		epochs := []*Measurement{
			{ID: "correlation-1", Source: SourceCorrelation, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{ID: "correlation-2", Source: SourceCorrelation, Symbol: "BTC/USD", At: time.Unix(2, 0)},
		}
		epoch := 0
		incoming := make([]*Measurement, 1)
		thesis.Measurements.Store(SourceCorrelation, []*Measurement{epochs[epoch]})

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			epoch = 1 - epoch
			incoming[0] = epochs[epoch]

			if err := thesis.AppendMeasurements(
				SourceCorrelation, incoming, false,
			); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkThesisFanout(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.ObserveCompletions(func(EpochObservation) {})
	sources := []SourceType{
		SourceCorrelation,
		SourceCVD,
		SourceDepthFlow,
		SourceExhaustion,
		SourceHawkes,
		SourceLeadLag,
		SourceLiquidity,
		SourcePumpDump,
		SourceSentiment,
		SourceToxicity,
		SourceCategories,
		SourceCognition,
		SourceManifold,
		SourceResonance,
		SourceCausal,
		SourceGraph,
		SourcePlanner,
	}

	for b.Loop() {
		for _, source := range sources {
			thesis.Stamp(source)
		}

		thesis.Fanout(SourcePlanner)
		thesis.Reset()
	}
}
