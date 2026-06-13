package manifold

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/internal/testconfig"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestLiquidityRho(t *testing.T) {
	convey.Convey("Given visible book liquidity", t, func() {
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 128)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should scale deposits against visible depth and rho_min", func() {
			convey.So(err, convey.ShouldBeNil)

			state := field.universe.loadSymbol("XBT/USD")
			state.bookReady = true
			state.book = krakenmarket.BookUpdate{
				Bids: []krakenmarket.BookLevel{{Price: 49990, Qty: 2}},
				Asks: []krakenmarket.BookLevel{{Price: 50010, Qty: 3}},
			}

			rho, rhoErr := field.liquidityRho(state, 2.5, 1)

			convey.So(rhoErr, convey.ShouldBeNil)
			convey.So(rho, convey.ShouldAlmostEqual, 0.5*field.config.RhoMin/float64(field.config.MaxModes), 0.0001)

			field.Close()
		})
	})
}

func TestFieldGlobalReadingStoredPerSymbol(t *testing.T) {
	convey.Convey("Given an integrated manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should store the same global reading for every symbol carrier", func() {
			convey.So(err, convey.ShouldBeNil)

			global := mkernel.Reading{
				PressureGradNorm: 1,
				CoherenceMag2:    2,
				GuidanceSpeed:    3,
				ViscosityProxy:   4,
			}

			field.lastReading = global
			field.readings.Store("XBT/USD", symbolReading{reading: global, price: 100, at: time.Now()})
			field.readings.Store("ETH/USD", symbolReading{reading: global, price: 50, at: time.Now()})

			btcReading, _, _, btcOk := field.Reading("XBT/USD")
			ethReading, _, _, ethOk := field.Reading("ETH/USD")

			convey.So(btcOk, convey.ShouldBeTrue)
			convey.So(ethOk, convey.ShouldBeTrue)
			convey.So(btcReading, convey.ShouldResemble, global)
			convey.So(ethReading, convey.ShouldResemble, global)

			field.Close()
		})
	})
}

func TestFieldFeedTradeWhaleParticle(t *testing.T) {
	convey.Convey("Given a manifold field", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should enqueue whale trades as PIC particles instead of grid deposits", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			field.lastStepAt = time.Now()

			state := field.universe.loadSymbol("XBT/USD")
			state.midPrice = 50000
			state.bookReady = true
			state.SetTradeQtys([]float64{0.1, 0.2, 0.15, 0.12, 0.18})
			state.SetReturns([]float64{0.01, -0.008, 0.012})

			smallErr := field.FeedTrade(&krakenmarket.TradeUpdate{
				Symbol: "XBT/USD",
				Price:  50010,
				Qty:    0.15,
				Side:   "buy",
			}, time.Now())

			convey.So(smallErr, convey.ShouldBeNil)
			convey.So(len(field.pendingDeposits), convey.ShouldEqual, 1)
			convey.So(len(field.pendingWhales), convey.ShouldEqual, 0)

			whaleErr := field.FeedTrade(&krakenmarket.TradeUpdate{
				Symbol: "XBT/USD",
				Price:  50010,
				Qty:    50,
				Side:   "buy",
			}, time.Now())

			convey.So(whaleErr, convey.ShouldBeNil)
			convey.So(len(field.pendingWhales), convey.ShouldEqual, 1)
			convey.So(field.pendingWhales[0].oscillator.VelX, convey.ShouldBeGreaterThan, 0)
			convey.So(field.pendingWhales[0].oscillator.PosY, convey.ShouldEqual, 0)

			field.Close()
		})
	})
}

func TestFieldIntegrateWhaleReadback(t *testing.T) {
	convey.Convey("Given a whale carrier integrated through Metal PIC", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should read whale positions back from the solver", func() {
			convey.So(err, convey.ShouldBeNil)

			field.RegisterSymbols([]string{"XBT/USD"})
			state := field.universe.loadSymbol("XBT/USD")
			state.midPrice = 50000
			state.SetTradeQtys([]float64{0.1, 0.2, 0.15, 0.12, 0.18})
			state.SetReturns([]float64{0.01, -0.008, 0.012})

			whaleRho, rhoErr := field.liquidityRho(state, 50, 1)

			convey.So(rhoErr, convey.ShouldBeNil)

			field.pendingWhales = []whaleCarrier{{
				symbol: "XBT/USD",
				oscillator: field.whaleOscillatorFromTrade(
					state,
					&krakenmarket.TradeUpdate{
						Symbol: "XBT/USD",
						Price:  50010,
						Qty:    50,
						Side:   "buy",
					},
					field.universe.coords(state, 0),
					whaleRho,
				),
			}}

			at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			field.lastStepAt = at.Add(-time.Second)

			stepped, integrateErr := field.integrate(at)

			convey.So(integrateErr, convey.ShouldBeNil)
			convey.So(stepped, convey.ShouldBeTrue)
			convey.So(len(field.activeWhales), convey.ShouldBeGreaterThan, 0)
			convey.So(len(field.lastCarriers), convey.ShouldBeGreaterThan, 0)

			whaleCount := 0

			for _, carrier := range field.lastCarriers {
				if carrier.role != "whale" {
					continue
				}

				whaleCount++
				convey.So(oscillatorStateFinite(carrier.oscillator), convey.ShouldBeTrue)
			}

			convey.So(whaleCount, convey.ShouldBeGreaterThan, 0)

			field.Close()
		})
	})
}

func TestFieldPublishSnapshot(t *testing.T) {
	convey.Convey("Given a field with snapshot publish interval", t, func() {
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.grid_x", 32)
		viper.Set("signals.manifold.grid_y", 3)
		viper.Set("signals.manifold.grid_z", 16)
		viper.Set("signals.manifold.max_modes", 32)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("signals.manifold.snapshot_interval", "500ms")
		viper.Set("market.book_depth_levels", 10)

		field, err := newField()

		convey.Convey("It should throttle ui snapshots to the configured interval", func() {
			convey.So(err, convey.ShouldBeNil)

			publishCount := 0
			field.SetSnapshotPublisher(func(time.Time) error {
				publishCount++

				return nil
			})

			field.lastStepAt = time.Now()
			field.lastCarriers = []fieldCarrier{{role: "symbol", symbol: "XBT/USD"}}
			field.lastReading = mkernel.Reading{PressureGradNorm: 1}

			firstAt := time.Now()
			secondAt := firstAt.Add(100 * time.Millisecond)

			convey.So(field.publishSnapshot(firstAt), convey.ShouldBeNil)
			convey.So(field.publishSnapshot(secondAt), convey.ShouldBeNil)
			convey.So(publishCount, convey.ShouldEqual, 1)

			field.Close()
		})
	})
}

func TestCapSolverCarriersPreservesSymbols(t *testing.T) {
	convey.Convey("Given symbols and hotter whales than max modes", t, func() {
		symbolOscillators := make([]mkernel.Oscillator, 4)
		symbolCarriers := make([]fieldCarrier, 4)

		for index := range symbolOscillators {
			symbolOscillators[index] = mkernel.Oscillator{Heat: 0.001}
			symbolCarriers[index] = fieldCarrier{
				role:       "symbol",
				symbol:     fmt.Sprintf("SYM-%d", index),
				oscillator: symbolOscillators[index],
			}
		}

		whaleOscillators := make([]mkernel.Oscillator, 40)
		whaleCarriers := make([]fieldCarrier, 40)

		for index := range whaleOscillators {
			whaleOscillators[index] = mkernel.Oscillator{Heat: float64(index + 1)}
			whaleCarriers[index] = fieldCarrier{
				role:       "whale",
				symbol:     "XBT/USD",
				oscillator: whaleOscillators[index],
			}
		}

		convey.Convey("It should keep every symbol carrier for the solver first", func() {
			trimmedOscillators, trimmedCarriers := capSolverCarriers(
				symbolOscillators,
				symbolCarriers,
				whaleOscillators,
				whaleCarriers,
				8,
			)

			convey.So(len(trimmedOscillators), convey.ShouldEqual, 8)
			convey.So(len(trimmedCarriers), convey.ShouldEqual, 8)

			for index := range symbolCarriers {
				convey.So(trimmedCarriers[index].role, convey.ShouldEqual, "symbol")
			}
		})
	})
}

func TestCapCarriers(t *testing.T) {
	convey.Convey("Given more carriers than max modes", t, func() {
		oscillators := make([]mkernel.Oscillator, 40)
		carriers := make([]fieldCarrier, 40)

		for index := range oscillators {
			heat := float64(index) * 0.01
			oscillators[index] = mkernel.Oscillator{Heat: heat}
			carriers[index] = fieldCarrier{
				role:       "symbol",
				symbol:     "SYM",
				oscillator: oscillators[index],
			}
		}

		convey.Convey("It should keep the hottest max modes", func() {
			trimmedOscillators, trimmedCarriers := capCarriers(oscillators, carriers, 32)

			convey.So(len(trimmedOscillators), convey.ShouldEqual, 32)
			convey.So(len(trimmedCarriers), convey.ShouldEqual, 32)
			convey.So(trimmedOscillators[0].Heat, convey.ShouldEqual, oscillators[39].Heat)
			convey.So(trimmedOscillators[31].Heat, convey.ShouldEqual, oscillators[8].Heat)
		})
	})
}

func TestNormalizeOscillatorsForSolver(t *testing.T) {
	rhoMin := 49.0
	oscillators := []mkernel.Oscillator{
		{Heat: 0, Amplitude: 10},
		{Heat: 1, Amplitude: 5},
	}

	normalized := normalizeOscillatorsForSolver(oscillators, rhoMin, 128)

	if len(normalized) != 2 {
		t.Fatalf("len = %d, want 2", len(normalized))
	}

	wantHeat := rhoMin / 128.0

	if normalized[0].Heat != wantHeat {
		t.Fatalf("heat[0] = %g, want %g", normalized[0].Heat, wantHeat)
	}

	wantAmplitude := math.Sqrt(wantHeat)

	if normalized[0].Amplitude != wantAmplitude {
		t.Fatalf("amplitude[0] = %g, want %g", normalized[0].Amplitude, wantAmplitude)
	}
}

func TestIntegrateWarmupCarrierGrowth(t *testing.T) {
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 32)
	viper.Set("signals.manifold.grid_x", 32)
	viper.Set("signals.manifold.grid_y", 3)
	viper.Set("signals.manifold.grid_z", 16)
	viper.Set("signals.manifold.max_modes", 128)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 10)

	field, err := newField()

	if err != nil {
		t.Fatal(err)
	}

	defer field.Close()

	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	feed := func(symbol string, price float64) error {
		return field.FeedBook(krakenmarket.BookUpdate{
			Symbol: symbol,
			Type:   "snapshot",
			Bids:   []krakenmarket.BookLevel{{Price: price - 0.01, Qty: 2}, {Price: price - 0.02, Qty: 2}},
			Asks:   []krakenmarket.BookLevel{{Price: price + 0.01, Qty: 2}, {Price: price + 0.02, Qty: 2}},
		}, at)
	}

	field.RegisterSymbols([]string{"SYM0/USD", "SYM1/USD", "SYM2/USD"})

	if err := feed("SYM0/USD", 100); err != nil {
		t.Fatal(err)
	}

	if _, err := field.integrate(at.Add(time.Second)); err != nil {
		t.Fatalf("warmup integrate: %v", err)
	}

	if err := feed("SYM1/USD", 101); err != nil {
		t.Fatal(err)
	}

	if err := feed("SYM2/USD", 102); err != nil {
		t.Fatal(err)
	}

	if _, err := field.integrate(at.Add(2 * time.Second)); err != nil {
		t.Fatalf("growth integrate: %v", err)
	}
}

func TestIntegrateScaledSymbolCount(t *testing.T) {
	for _, symbolCount := range []int{1, 2, 3, 4, 5, 6, 7, 8, 16, 32, 64, 128} {
		t.Run(fmt.Sprintf("count=%d", symbolCount), func(t *testing.T) {
			t.Cleanup(func() {
				viper.Reset()
				testconfig.SeedRegimeDefaults()
			})

			testconfig.SeedRegimeDefaults()
			viper.Set("signals.manifold.tick_size", 0.01)
			viper.Set("signals.manifold.grid_half_width", 32)
			viper.Set("signals.manifold.grid_x", 32)
			viper.Set("signals.manifold.grid_y", 3)
			viper.Set("signals.manifold.grid_z", 16)
			viper.Set("signals.manifold.max_modes", 128)
			viper.Set("signals.manifold.integration_interval", "100ms")
			viper.Set("market.book_depth_levels", 10)

			field, err := newField()

			if err != nil {
				t.Fatal(err)
			}

			defer field.Close()

			symbols := make([]string, symbolCount)

			for index := range symbols {
				symbols[index] = fmt.Sprintf("SYM%d/USD", index)
			}

			field.RegisterSymbols(symbols)

			at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			field.lastStepAt = at

			for index, symbol := range symbols {
				price := 100.0 + float64(index)
				bids := []krakenmarket.BookLevel{
					{Price: price - 0.01, Qty: 2},
					{Price: price - 0.02, Qty: 2},
				}
				asks := []krakenmarket.BookLevel{
					{Price: price + 0.01, Qty: 2},
					{Price: price + 0.02, Qty: 2},
				}

				if feedErr := field.FeedBook(krakenmarket.BookUpdate{
					Symbol: symbol, Type: "snapshot", Bids: bids, Asks: asks, Timestamp: at,
				}, at); feedErr != nil {
					t.Fatalf("feed: %v", feedErr)
				}
			}

			_, integrateErr := field.integrate(at.Add(time.Second))

			if integrateErr != nil {
				t.Fatalf("integrate: %v", integrateErr)
			}
		})
	}
}

func TestIntegrateManySymbolBookDeposits(t *testing.T) {
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 32)
	viper.Set("signals.manifold.grid_x", 32)
	viper.Set("signals.manifold.grid_y", 3)
	viper.Set("signals.manifold.grid_z", 16)
	viper.Set("signals.manifold.max_modes", 128)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 10)

	field, err := newField()

	if err != nil {
		t.Fatal(err)
	}

	defer field.Close()

	symbols := make([]string, 648)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("SYM%d/USD", index)
	}

	field.RegisterSymbols(symbols)

	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	field.lastStepAt = at

	for index, symbol := range symbols {
		price := 1.0 + float64(index)*0.01
		bids := make([]krakenmarket.BookLevel, 10)
		asks := make([]krakenmarket.BookLevel, 10)

		for level := 0; level < 10; level++ {
			bids[level] = krakenmarket.BookLevel{
				Price: price - float64(level+1)*0.01,
				Qty:   1 + float64(level),
			}
			asks[level] = krakenmarket.BookLevel{
				Price: price + float64(level+1)*0.01,
				Qty:   1 + float64(level),
			}
		}

		feedErr := field.FeedBook(krakenmarket.BookUpdate{
			Symbol:    symbol,
			Type:      "snapshot",
			Bids:      bids,
			Asks:      asks,
			Timestamp: at,
		}, at)

		if feedErr != nil {
			t.Fatalf("feed %s: %v", symbol, feedErr)
		}
	}

	stepped, integrateErr := field.integrate(at.Add(time.Second))

	if integrateErr != nil {
		t.Fatalf("integrate: %v", integrateErr)
	}

	if !stepped {
		t.Fatal("expected integrate to step")
	}
}

func TestIntegrateWrongTickSizeDeposit(t *testing.T) {
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 32)
	viper.Set("signals.manifold.grid_x", 32)
	viper.Set("signals.manifold.grid_y", 3)
	viper.Set("signals.manifold.grid_z", 16)
	viper.Set("signals.manifold.max_modes", 128)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 10)

	field, err := newField()

	if err != nil {
		t.Fatal(err)
	}

	defer field.Close()

	symbols := make([]string, 128)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ALT%d/USD", index)
	}

	field.RegisterSymbols(symbols)

	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	field.lastStepAt = at

	for index, symbol := range symbols {
		microPrice := 0.00001 * float64(index+1)
		bids := []krakenmarket.BookLevel{{Price: microPrice * 0.99, Qty: 1000}}
		asks := []krakenmarket.BookLevel{{Price: microPrice * 1.01, Qty: 1000}}

		feedErr := field.FeedBook(krakenmarket.BookUpdate{
			Symbol:    symbol,
			Type:      "snapshot",
			Bids:      bids,
			Asks:      asks,
			Timestamp: at,
		}, at)

		if feedErr != nil {
			t.Fatalf("feed %s: %v", symbol, feedErr)
		}
	}

	_, integrateErr := field.integrate(at.Add(time.Second))

	if integrateErr != nil {
		t.Fatalf("integrate with micro prices and coarse tick fallback: %v", integrateErr)
	}
}
