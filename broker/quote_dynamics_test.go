package broker

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func TestQuoteDynamicsRegistrySpreadLimit(test *testing.T) {
	testconfig.Load(test)

	registry := NewQuoteDynamicsRegistry()
	now := time.Now().UTC()
	envelope, envelopeErr := symmmarket.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		test.Fatalf("dynamics envelope: %v", envelopeErr)
	}

	for range envelope.MinSamples {
		registry.Record(QuoteSnapshot{
			Symbol:     "ALICE/USD",
			Bid:        99.5,
			Ask:        100.5,
			ObservedAt: now,
		})
	}

	limit, limitErr := registry.SpreadLimitBps("ALICE/USD")

	convey.Convey("Given a symbol with a wide but stable spread history", test, func() {
		convey.Convey("It should derive a positive per-symbol ceiling", func() {
			convey.So(limitErr, convey.ShouldBeNil)
			convey.So(limit, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestPreTradeRiskGateAllowsSymbolTypicalSpread(test *testing.T) {
	testconfig.Load(test)

	tradingConfig := config.TradingConfig{
		Model:                  "paper",
		MaxConcurrentPositions: 1,
		MaxQuoteAge:            time.Minute,
		OrderAckTimeout:        time.Second,
		EntryTransitTTL:        time.Second,
	}
	riskGate, gateErr := NewPreTradeRiskGate(tradingConfig)

	if gateErr != nil {
		test.Fatalf("risk gate: %v", gateErr)
	}

	now := time.Now().UTC()
	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     trading.Buy,
		Symbol:   "ALICE/USD",
		Price:    100,
		Quantity: 1,
	}

	seedSpreadHistory(riskGate, "ALICE/USD", 99.38, 100.62, now)

	convey.Convey("Given a spread within the symbol baseline", test, func() {
		quote := QuoteSnapshot{
			Symbol:     "ALICE/USD",
			Bid:        99.385,
			Ask:        100.615,
			Last:       100,
			ObservedAt: now,
		}

		convey.Convey("It should accept the entry", func() {
			convey.So(riskGate.Validate(action, quote, now), convey.ShouldBeNil)
		})
	})
}

func TestPreTradeRiskGateRejectsSpreadAnomaly(test *testing.T) {
	testconfig.Load(test)

	tradingConfig := config.TradingConfig{
		Model:                  "paper",
		MaxConcurrentPositions: 1,
		MaxQuoteAge:            time.Minute,
		OrderAckTimeout:        time.Second,
		EntryTransitTTL:        time.Second,
	}
	riskGate, gateErr := NewPreTradeRiskGate(tradingConfig)

	if gateErr != nil {
		test.Fatalf("risk gate: %v", gateErr)
	}

	now := time.Now().UTC()
	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     trading.Buy,
		Symbol:   "BTC/USD",
		Price:    100,
		Quantity: 0.1,
	}

	seedSpreadHistory(riskGate, "BTC/USD", 99.99, 100.01, now)

	convey.Convey("Given a tight symbol with a sudden spread blowout", test, func() {
		quote := QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99,
			Ask:        101,
			Last:       100,
			ObservedAt: now,
		}

		convey.Convey("It should reject the entry", func() {
			convey.So(riskGate.Validate(action, quote, now), convey.ShouldNotBeNil)
		})
	})
}

func BenchmarkQuoteDynamicsRecord(b *testing.B) {
	testconfig.MustLoad()

	registry := NewQuoteDynamicsRegistry()
	quote := QuoteSnapshot{
		Symbol:     "BTC/USD",
		Bid:        99.99,
		Ask:        100.01,
		Last:       100,
		ObservedAt: time.Now().UTC(),
	}

	b.ReportAllocs()

	for b.Loop() {
		registry.Record(quote)
	}
}

func BenchmarkQuoteDynamicsSpreadLimit(b *testing.B) {
	testconfig.MustLoad()

	registry := NewQuoteDynamicsRegistry()
	now := time.Now().UTC()
	envelope, envelopeErr := symmmarket.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		b.Fatal(envelopeErr)
	}

	for range envelope.WindowCapacity {
		registry.Record(QuoteSnapshot{
			Symbol:     "BTC/USD",
			Bid:        99.99,
			Ask:        100.01,
			Last:       100,
			ObservedAt: now,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = registry.SpreadLimitBps("BTC/USD")
	}
}
