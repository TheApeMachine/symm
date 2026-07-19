package fluid

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, instrument, channel)}
}

func withFluidInterval(t testing.TB) {
	t.Helper()

	previous := viper.Get("signals.fluid.integration_interval")
	t.Cleanup(func() {
		viper.Set("signals.fluid.integration_interval", previous)
	})
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
}

/*
TestSignal_MeasureFromMarket proves fluid on the mock Conn Session path:
exhaustion/vacuum tapes must emit SourceFluid viscosity and laminar family
peaks, and exhaustion viscosity must exceed calm.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	withFluidInterval(t)

	symbol := conditions.Subject()
	options := tests.SessionOptions{Signals: sessionSignals}

	// Viscosity saturates on calm and stressed Session books; Reynolds and
	// turbulent score are the decay-sensitive peaks that diverge.
	t.Run("tape_exhaustion", func(t *testing.T) {
		tests.PlayMarketClaims(t, options, conditions.TapeExhaustion().Frames(),
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricViscosity,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricLaminarScore,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricReynolds,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricMemory,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
	})

	t.Run("tape_vacuum", func(t *testing.T) {
		tests.PlayMarketClaims(t, options, conditions.TapeVacuum().Frames(),
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricViscosity,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricTurbulentScore,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricReynolds,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
	})

	t.Run("exhaustion_reynolds_exceeds_calm", func(t *testing.T) {
		calm := tests.PlayMarketClaims(t, options, conditions.TapeCalm().Frames(),
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricReynolds,
				Symbol: symbol, Bound: tests.BoundZero,
			},
		)
		hot := tests.PlayMarketClaims(t, options, conditions.TapeExhaustion().Frames(),
			tests.SourceClaim{
				Source: types.SourceFluid, Metric: types.MetricReynolds,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
		tests.RequireSourceExceeds(
			t, hot, calm,
			types.SourceFluid, symbol, types.MetricReynolds,
		)
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	withFluidInterval(benchmark)

	session, err := tests.NewSession(context.Background(), benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	frames := conditions.TapeExhaustion().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	previousInterval := viper.Get("signals.fluid.integration_interval")
	benchmark.Cleanup(func() {
		viper.Set("signals.fluid.integration_interval", previousInterval)
	})
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
	signal := NewSignal(context.Background(), nil, nil, nil)
	at := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fixture := &symbolBookFixture{symbol: "BTC/USD"}

	for index := 0; index < 4; index++ {
		stamp := at.Add(time.Duration(index) * 200 * time.Millisecond)
		ticker := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(99.5),
			Ask:       krakendecimal.NewFromFloat64(100.5),
			Last:      krakendecimal.NewFromFloat64(100),
			Volume:    1_000,
			Timestamp: stamp,
		}
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
		row := fixture.snapshot(99.5, 20-float64(index), 100.5, 10)
		row.Timestamp = stamp
		row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			Books:        []kraken.BookData{row},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
	}

	stamp := at.Add(time.Second)
	ticker := kraken.TickerData{
		Symbol:    "BTC/USD",
		Bid:       krakendecimal.NewFromFloat64(99.5),
		Ask:       krakendecimal.NewFromFloat64(100.5),
		Last:      krakendecimal.NewFromFloat64(100),
		Volume:    1_200,
		Timestamp: stamp,
	}
	row := fixture.snapshot(99.5, 12, 100.5, 14)
	row.Timestamp = stamp
	row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
	frame := &types.MarketFrame{
		Tickers:      []kraken.TickerData{ticker},
		Books:        []kraken.BookData{row},
		CrossSection: types.NewCrossSection(),
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
