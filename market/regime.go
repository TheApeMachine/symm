package market

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

const MarketRegimeSymbol = "market"

type RegimeConfig struct {
	Window       int
	MinSamples   int
	AnchorSymbol string
}

func (cfg *RegimeConfig) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"window":      cfg.Window,
		"min_samples": cfg.MinSamples,
	}))
}

type RegimeStrengths struct {
	Volatility float64
	Trend      float64
	Bullish    float64
	Bearish    float64
	Choppiness float64
}

type regimeMetrics struct {
	volatility     float64
	trendScore     float64
	bullishRaw     float64
	bearishRaw     float64
	signChangeRate float64
}

type RegimeDynamics struct {
	volScale         *Baseline
	trendScore       *Baseline
	trendSigma       float64
	strongTrendSigma float64
	volFloorSigma    float64
	volScaleFloor    float64
}

/*
RegimeClassifier tracks per-symbol return windows and derives cross-section
mean regime strengths for the dashboard spider chart.
*/
type RegimeClassifier struct {
	config       RegimeConfig
	crossSection *CrossSection
	adaptation   *AdaptationController
}

func RegimeConfigFromViper() (*RegimeConfig, error) {
	regime, err := config.DerivedRegimeSpec()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return config.NewSafeConfig(&RegimeConfig{
		Window:       regime.Window,
		MinSamples:   regime.MinSamples,
		AnchorSymbol: viper.GetString("market.anchor_symbol"),
	})
}

func NewRegimeClassifier(crossSection *CrossSection) (*RegimeClassifier, error) {
	regimeConfig, err := RegimeConfigFromViper()

	if err != nil {
		return nil, errnie.Error(err)
	}

	if regimeConfig.Window <= 0 {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"regime.window": regimeConfig.Window,
		}))
	}

	if regimeConfig.MinSamples <= 0 {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"regime.min_samples": regimeConfig.MinSamples,
		}))
	}

	if crossSection == nil {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"cross_section": crossSection,
		}))
	}

	adaptationController, adaptationErr := LoadAdaptation()

	if adaptationErr != nil {
		return nil, errnie.Error(adaptationErr)
	}

	return &RegimeClassifier{
		config:       *regimeConfig,
		crossSection: crossSection,
		adaptation:   adaptationController,
	}, nil
}

func (classifier *RegimeClassifier) Observe(measurement logic.Measurement) error {
	if err := measurement.Market.Validate(); err != nil {
		return fmt.Errorf("market: regime %s: %w", measurement.Symbol, err)
	}

	return classifier.crossSection.Observe(&measurement.Market)
}

func (classifier *RegimeClassifier) SymbolVolatility(symbol string) float64 {
	stopPct, _, ok := classifier.SymbolExitMoves(symbol)

	if !ok {
		return 0
	}

	return stopPct
}

func (classifier *RegimeClassifier) SymbolExitMoves(symbol string) (stopPct float64, profitPct float64, ok bool) {
	if classifier == nil || classifier.crossSection == nil || symbol == "" {
		return 0, 0, false
	}

	returns := classifier.crossSection.SymbolReturns(symbol, classifier.config.Window)

	if len(returns) < classifier.config.MinSamples {
		return 0, 0, false
	}

	stopPct = float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(returns...)...))

	if stopPct <= 0 {
		stopPct = returnVolatility(returns)
	}

	if stopPct <= 0 {
		return 0, 0, false
	}

	positives := make([]float64, 0, len(returns))

	for _, value := range returns {
		if value > 0 {
			positives = append(positives, value)
		}
	}

	if len(positives) >= classifier.config.MinSamples/2 {
		profitPct = float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(positives...)...))
	}

	if profitPct <= 0 {
		profitPct = stopPct
	}

	return stopPct, profitPct, true
}

func (classifier *RegimeClassifier) MarketMean() (RegimeStrengths, bool) {
	dynamics := classifier.adaptation.RegimeDynamics()
	minSamples := classifier.config.MinSamples
	window := classifier.config.Window
	now := time.Now()

	returns := classifier.crossSection.marketMedianReturns(now, window, minSamples)

	if len(returns) < minSamples {
		anchor := classifier.config.AnchorSymbol

		if anchor != "" {
			returns = classifier.crossSection.trailingSymbolReturns(anchor, window)
		}
	}

	metrics, ok := extractRegimeMetrics(returns, minSamples)

	if !ok {
		return RegimeStrengths{}, false
	}

	classifier.adaptation.ObserveRegimeSamples(
		[]float64{metrics.volatility},
		[]float64{metrics.trendScore},
	)

	return classifyMetrics(metrics, dynamics), true
}

func extractRegimeMetrics(returns []float64, minSamples int) (regimeMetrics, bool) {
	if len(returns) < minSamples {
		return regimeMetrics{}, false
	}

	volatility := returnVolatility(returns)

	if volatility <= 0 {
		return regimeMetrics{}, false
	}

	sampleCount := float64(len(returns))
	cumulative := 0.0

	for _, value := range returns {
		cumulative += value
	}

	trendScore := math.Abs(cumulative) / (volatility * math.Sqrt(sampleCount))

	bullishRaw := 0.0
	bearishRaw := 0.0

	if cumulative > 0 {
		bullishRaw = cumulative / sampleCount
	}

	if cumulative < 0 {
		bearishRaw = -cumulative / sampleCount
	}

	return regimeMetrics{
		volatility:     volatility,
		trendScore:     trendScore,
		bullishRaw:     bullishRaw,
		bearishRaw:     bearishRaw,
		signChangeRate: returnSignChangeRate(returns),
	}, true
}

func classifyMetrics(metrics regimeMetrics, dynamics RegimeDynamics) RegimeStrengths {
	volScale := dynamics.volScaleFloor

	if dynamics.volScale != nil && dynamics.volScale.Ready() {
		volScale = dynamics.volScale.Scale()
	}

	volStrength := metrics.volatility / volScale

	if dynamics.volScale != nil && dynamics.volScale.Ready() {
		volZScore, ok := dynamics.volScale.ZScore(metrics.volatility, dynamics.volFloorSigma)

		if ok && volZScore < -dynamics.volFloorSigma {
			return RegimeStrengths{Volatility: volStrength}
		}
	}

	trend := 0.0

	if dynamics.trendScore != nil && dynamics.trendScore.Ready() {
		trendZScore, ok := dynamics.trendScore.ZScore(metrics.trendScore, 0)

		if ok && trendZScore > dynamics.trendSigma {
			excess := trendZScore - dynamics.trendSigma

			if dynamics.strongTrendSigma > 0 {
				trend = excess / dynamics.strongTrendSigma
			}
		}
	}

	bullish := metrics.bullishRaw / volScale
	bearish := metrics.bearishRaw / volScale
	choppiness := volStrength * (1 - trend)

	if choppiness <= 0 {
		choppiness = metrics.signChangeRate * volStrength
	}

	return RegimeStrengths{
		Volatility: volStrength,
		Trend:      trend,
		Bullish:    bullish,
		Bearish:    bearish,
		Choppiness: choppiness,
	}
}

func classifyReturns(returns []float64, minSamples int, dynamics RegimeDynamics) (RegimeStrengths, error) {
	metrics, ok := extractRegimeMetrics(returns, minSamples)

	if !ok {
		return RegimeStrengths{}, nil
	}

	return classifyMetrics(metrics, dynamics), nil
}

func returnVolatility(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	mean := 0.0

	for _, value := range returns {
		mean += value
	}

	mean /= float64(len(returns))

	variance := 0.0

	for _, value := range returns {
		delta := value - mean
		variance += delta * delta
	}

	variance /= float64(len(returns))

	if variance <= 0 {
		return 0
	}

	return math.Sqrt(variance)
}

func returnSignChangeRate(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	changes := 0

	for index := 1; index < len(returns); index++ {
		prior := returns[index-1]
		current := returns[index]

		if prior == 0 || current == 0 {
			continue
		}

		if math.Signbit(prior) != math.Signbit(current) {
			changes++
		}
	}

	return float64(changes) / float64(len(returns)-1)
}

func (classifier *RegimeClassifier) PublishFrame(
	bus *internal.Bus,
	mean RegimeStrengths,
	ready bool,
) error {
	if bus == nil {
		return errnie.Error(errors.New("market: publish frame bus is nil"))
	}

	if !ready {
		return nil
	}

	return bus.Send(internal.ChannelUI, "regime", map[string]any{
		"chart":      "regime",
		"symbol":     MarketRegimeSymbol,
		"volatility": mean.Volatility,
		"trend":      mean.Trend,
		"bullish":    mean.Bullish,
		"bearish":    mean.Bearish,
		"choppiness": mean.Choppiness,
	})
}
