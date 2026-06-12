package market

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func configureRegimeViper() {
	viper.Set("regime.window", 16)
	viper.Set("regime.min_samples", 4)
	viper.Set("regime.baseline.alpha_min", 0.05)
	viper.Set("regime.baseline.alpha_max", 0.25)
	viper.Set("regime.baseline.min_obs", 4)
	viper.Set("regime.baseline.trend_sigma", 1.0)
	viper.Set("regime.baseline.strong_trend_sigma", 2.0)
	viper.Set("regime.baseline.vol_floor_sigma", 3.0)
	viper.Set("regime.baseline.vol_scale_floor", 0.000001)
	viper.Set("regime.baseline.seed_vol_scale", 0.0002)
	viper.Set("signals.causal.contagion_window_slow_max", 128)
	viper.Set("signals.causal.contagion_window_slow_min", 16)
	viper.Set("market.anchor_symbol", "BTC/EUR")
}

func observeRegimeMeasurement(
	classifier *RegimeClassifier,
	symbol string,
	price float64,
	eventAt time.Time,
) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, 0.01, price*1000, 1, eventAt)

	if err != nil {
		panic(err)
	}

	if err := classifier.Observe(logic.Measurement{
		Symbol:     symbol,
		Price:      price,
		ObservedAt: eventAt,
		Market:     *row,
	}); err != nil {
		panic(err)
	}
}

func observeRegimePrices(
	classifier *RegimeClassifier,
	symbol string,
	startPrice float64,
	step float64,
	returnCount int,
	baseUnix int64,
) {
	observeRegimeMeasurement(
		classifier,
		symbol,
		startPrice,
		time.Unix(baseUnix, 0),
	)

	for index := range returnCount {
		observeRegimeMeasurement(
			classifier,
			symbol,
			startPrice+step*float64(index+1),
			time.Unix(baseUnix+10+int64(index), 0),
		)
	}
}

func warmedRegimeDynamics() RegimeDynamics {
	volBaseline := NewBaseline(0.000001, 4)
	trendBaseline := NewBaseline(0.05, 4)

	for range 16 {
		_ = volBaseline.Observe(0.0002, 0.1)
		_ = trendBaseline.Observe(1.0, 0.1)
	}

	return RegimeDynamics{
		volScale:         volBaseline,
		trendScore:       trendBaseline,
		trendSigma:       1.0,
		strongTrendSigma: 2.0,
		volFloorSigma:    3.0,
		volScaleFloor:    0.000001,
	}
}

func TestClassifyReturns(t *testing.T) {
	Convey("Given adaptive regime dynamics", t, func() {
		dynamics := warmedRegimeDynamics()

		Convey("It should classify a bullish trend", func() {
			returns := []float64{0.002, 0.0025, 0.0018, 0.0022, 0.0021}
			strengths, err := classifyReturns(returns, 4, dynamics)
			So(err, ShouldBeNil)

			So(strengths.Volatility, ShouldBeGreaterThan, 0)
			So(strengths.Trend, ShouldBeGreaterThan, 0)
			So(strengths.Bullish, ShouldBeGreaterThan, strengths.Bearish)
		})

		Convey("It should classify a choppy tape", func() {
			returns := []float64{0.004, -0.0035, 0.003, -0.004, 0.0035}
			strengths, err := classifyReturns(returns, 4, dynamics)
			So(err, ShouldBeNil)

			So(strengths.Volatility, ShouldBeGreaterThan, 0)
			So(strengths.Choppiness, ShouldBeGreaterThan, 0)
		})

		Convey("It should reject windows below min samples", func() {
			strengths, err := classifyReturns([]float64{0.001, 0.002}, 4, dynamics)
			So(err, ShouldBeNil)

			So(strengths, ShouldResemble, RegimeStrengths{})
		})
	})
}

func TestRegimeClassifierMarketMeanNotReady(t *testing.T) {
	Convey("Given symbols still warming up", t, func() {
		configureRegimeViper()

		crossSection := &CrossSection{returnCap: 32}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		observeRegimeMeasurement(
			classifier,
			"BTC/EUR",
			100,
			time.Unix(1_700_000_000, 0),
		)

		_, ready := classifier.MarketMean()

		Convey("It should defer without treating warmup as a fault", func() {
			So(ready, ShouldBeFalse)
		})
	})
}

func TestRegimeClassifierMarketMean(t *testing.T) {
	Convey("Given symbol return histories", t, func() {
		configureRegimeViper()

		crossSection := &CrossSection{returnCap: 32}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		observeRegimeMeasurement(
			classifier,
			"BTC/EUR",
			100,
			time.Unix(1_700_000_000, 0),
		)

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)
		observeRegimePrices(classifier, "ETH/EUR", 50, -0.2, 16, 1_700_000_030)

		mean, ready := classifier.MarketMean()
		So(ready, ShouldBeTrue)

		Convey("It should classify one cross-section median return series", func() {
			So(mean.Volatility, ShouldBeGreaterThan, 0)
			So(mean.Bullish+mean.Bearish, ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegimeClassifierMarketMeanOneSymbol(t *testing.T) {
	Convey("Given only the anchor symbol warmed", t, func() {
		configureRegimeViper()

		crossSection := &CrossSection{returnCap: 32}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		observeRegimeMeasurement(
			classifier,
			"BTC/EUR",
			100,
			time.Unix(1_700_000_000, 0),
		)

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)

		mean, ready := classifier.MarketMean()

		Convey("It should publish regime from one cross-section read", func() {
			So(ready, ShouldBeTrue)
			So(mean.Volatility, ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegimeClassifierPublishFrame(t *testing.T) {
	Convey("Given a ui bus", t, func() {
		configureRegimeViper()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		bus := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
		subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})

		crossSection := &CrossSection{returnCap: 32}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		observeRegimeMeasurement(
			classifier,
			"BTC/EUR",
			100,
			time.Unix(1_700_000_000, 0),
		)

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)

		So(classifier.PublishFrame(bus), ShouldBeNil)

		frame, receiveErr := subscriber.Receive(internal.ChannelUI)

		So(receiveErr, ShouldBeNil)

		payload, ok := frame.Value.(map[string]any)

		So(ok, ShouldBeTrue)
		So(payload["chart"], ShouldEqual, "regime")
		So(payload["symbol"], ShouldEqual, MarketRegimeSymbol)
		So(payload["volatility"], ShouldBeGreaterThan, 0)
	})
}

func TestRegimeClassifierMarketMeanWithLargeWindow(t *testing.T) {
	Convey("Given regime.window larger than return capacity", t, func() {
		configureRegimeViper()
		viper.Set("regime.window", 256)

		crossSection := &CrossSection{returnCap: 64}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		observeRegimeMeasurement(
			classifier,
			"BTC/EUR",
			100,
			time.Unix(1_700_000_000, 0),
		)

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)
		observeRegimePrices(classifier, "ETH/EUR", 50, -0.2, 16, 1_700_000_030)

		mean, ready := classifier.MarketMean()
		So(ready, ShouldBeTrue)

		Convey("It should still classify from available returns", func() {
			So(mean.Volatility, ShouldBeGreaterThan, 0)
			So(mean.Bullish+mean.Bearish, ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegimeBaselineShift(t *testing.T) {
	Convey("Given a volatility regime break", t, func() {
		configureRegimeViper()

		controller, err := NewAdaptationController()

		So(err, ShouldBeNil)

		for range 32 {
			controller.ObserveRegimeSamples([]float64{0.0002}, []float64{1.0})
		}

		dynamics := controller.RegimeDynamics()
		quietTrend, err := classifyReturns(
			[]float64{0.002, 0.0025, 0.0018, 0.0022, 0.0021},
			4,
			dynamics,
		)

		So(err, ShouldBeNil)

		for range 32 {
			controller.ObserveRegimeSamples([]float64{0.01}, []float64{12.0})
		}

		dynamics = controller.RegimeDynamics()
		loudNoise, err := classifyReturns(
			[]float64{0.002, 0.0025, 0.0018, 0.0022, 0.0021},
			4,
			dynamics,
		)

		So(err, ShouldBeNil)

		Convey("It should re-anchor trend strength after the shift", func() {
			So(quietTrend.Trend, ShouldBeGreaterThan, 0)
			So(loudNoise.Trend, ShouldBeLessThan, quietTrend.Trend)
		})
	})
}

func BenchmarkMarketMean(b *testing.B) {
	configureRegimeViper()

	crossSection := &CrossSection{returnCap: 256}
	classifier, err := NewRegimeClassifier(crossSection)

	if err != nil {
		b.Fatal(err)
	}

	for symbolIndex := range 128 {
		symbol := fmt.Sprintf("SYM%d/EUR", symbolIndex)
		observeRegimePrices(classifier, symbol, 100+float64(symbolIndex), 0.01, 16, 1_700_000_000+int64(symbolIndex*20))
	}

	b.ReportAllocs()

	for b.Loop() {
		_, ready := classifier.MarketMean()

		if !ready {
			b.Fatal("market mean not ready")
		}
	}
}

func BenchmarkClassifyReturns(b *testing.B) {
	dynamics := warmedRegimeDynamics()
	returns := make([]float64, 256)

	for index := range returns {
		if index%2 == 0 {
			returns[index] = 0.002
			continue
		}

		returns[index] = -0.0015
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := classifyReturns(returns, 16, dynamics)

		if err != nil {
			b.Fatal(err)
		}
	}
}
