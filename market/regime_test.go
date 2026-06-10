package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric/adaptive"
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
}

func observeRegimePrices(
	classifier *RegimeClassifier,
	symbol string,
	startPrice float64,
	step float64,
	returnCount int,
	baseUnix int64,
) {
	classifier.Observe(logic.Measurement{
		Symbol:     symbol,
		Price:      startPrice,
		ObservedAt: time.Unix(baseUnix, 0),
	})

	for index := range returnCount {
		classifier.Observe(logic.Measurement{
			Symbol:     symbol,
			Price:      startPrice + step*float64(index+1),
			ObservedAt: time.Unix(baseUnix+10+int64(index), 0),
		})
	}
}

func warmedRegimeDynamics() RegimeDynamics {
	volBaseline := adaptive.NewBaseline(0.000001, 4)
	trendBaseline := adaptive.NewBaseline(0.05, 4)

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
			strengths := classifyReturns(returns, 4, dynamics)

			So(strengths.Volatility, ShouldBeGreaterThan, 0)
			So(strengths.Trend, ShouldBeGreaterThan, 0)
			So(strengths.Bullish, ShouldBeGreaterThan, strengths.Bearish)
		})

		Convey("It should classify a choppy tape", func() {
			returns := []float64{0.004, -0.0035, 0.003, -0.004, 0.0035}
			strengths := classifyReturns(returns, 4, dynamics)

			So(strengths.Volatility, ShouldBeGreaterThan, 0)
			So(strengths.Choppiness, ShouldBeGreaterThan, 0)
		})

		Convey("It should reject windows below min samples", func() {
			strengths := classifyReturns([]float64{0.001, 0.002}, 4, dynamics)

			So(strengths, ShouldResemble, RegimeStrengths{})
		})
	})
}

func TestRegimeClassifierMarketMean(t *testing.T) {
	Convey("Given symbol return histories", t, func() {
		configureRegimeViper()

		crossSection := &CrossSection{returnCap: 32}
		classifier, err := NewRegimeClassifier(crossSection)

		So(err, ShouldBeNil)

		classifier.Observe(logic.Measurement{
			Symbol:     "BTC/EUR",
			Price:      100,
			ObservedAt: time.Unix(1_700_000_000, 0),
		})

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)
		observeRegimePrices(classifier, "ETH/EUR", 50, -0.2, 16, 1_700_000_030)

		mean := classifier.MarketMean()

		Convey("It should average strengths across symbols", func() {
			So(mean.Volatility, ShouldBeGreaterThan, 0)
			So(mean.Bullish+mean.Bearish, ShouldBeGreaterThan, 0)
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

		classifier.Observe(logic.Measurement{
			Symbol:     "BTC/EUR",
			Price:      100,
			ObservedAt: time.Unix(1_700_000_000, 0),
		})

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

		classifier.Observe(logic.Measurement{
			Symbol:     "BTC/EUR",
			Price:      100,
			ObservedAt: time.Unix(1_700_000_000, 0),
		})

		observeRegimePrices(classifier, "BTC/EUR", 100, 0.2, 16, 1_700_000_010)
		observeRegimePrices(classifier, "ETH/EUR", 50, -0.2, 16, 1_700_000_030)

		mean := classifier.MarketMean()

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
		quietTrend := classifyReturns(
			[]float64{0.002, 0.0025, 0.0018, 0.0022, 0.0021},
			4,
			dynamics,
		)

		for range 32 {
			controller.ObserveRegimeSamples([]float64{0.01}, []float64{12.0})
		}

		dynamics = controller.RegimeDynamics()
		loudNoise := classifyReturns(
			[]float64{0.002, 0.0025, 0.0018, 0.0022, 0.0021},
			4,
			dynamics,
		)

		Convey("It should re-anchor trend strength after the shift", func() {
			So(quietTrend.Trend, ShouldBeGreaterThan, 0)
			So(loudNoise.Trend, ShouldBeLessThan, quietTrend.Trend)
		})
	})
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
		_ = classifyReturns(returns, 16, dynamics)
	}
}
