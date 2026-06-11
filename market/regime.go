package market

import (
	"errors"
	"fmt"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const MarketRegimeSymbol = "market"

type RegimeConfig struct {
	Window     int
	MinSamples int
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
	volScale         *adaptive.Baseline
	trendScore       *adaptive.Baseline
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
	return config.NewSafeConfig(&RegimeConfig{
		Window:     viper.GetInt("regime.window"),
		MinSamples: viper.GetInt("regime.min_samples"),
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

	stopPct = numeric.MedianAbsolute(returns)

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
		profitPct = numeric.Median(positives)
	}

	if profitPct <= 0 {
		profitPct = stopPct
	}

	return stopPct, profitPct, true
}

func (classifier *RegimeClassifier) MarketMean() (RegimeStrengths, error) {
	dynamics := classifier.adaptation.RegimeDynamics()
	mean := RegimeStrengths{}
	sampleCount := 0
	volatilities := make([]float64, 0, 8)
	trendScores := make([]float64, 0, 8)

	classifier.crossSection.eachSymbolReturns(
		classifier.config.Window,
		func(_ string, returns []float64) {
			metrics, err := extractRegimeMetrics(returns, classifier.config.MinSamples)

			if err != nil {
				errnie.Error(err)
				return
			}

			volatilities = append(volatilities, metrics.volatility)
			trendScores = append(trendScores, metrics.trendScore)

			strengths := classifyMetrics(metrics, dynamics)

			if strengths.Volatility == 0 &&
				strengths.Trend == 0 &&
				strengths.Bullish == 0 &&
				strengths.Bearish == 0 &&
				strengths.Choppiness == 0 {
				return
			}

			mean.Volatility += strengths.Volatility
			mean.Trend += strengths.Trend
			mean.Bullish += strengths.Bullish
			mean.Bearish += strengths.Bearish
			mean.Choppiness += strengths.Choppiness
			sampleCount++
		},
	)

	classifier.adaptation.ObserveRegimeSamples(volatilities, trendScores)

	if sampleCount == 0 {
		return RegimeStrengths{}, errnie.Error(
			errors.New("market: market mean: sample count is 0"),
		)
	}

	divisor := float64(sampleCount)

	return RegimeStrengths{
		Volatility: mean.Volatility / divisor,
		Trend:      mean.Trend / divisor,
		Bullish:    mean.Bullish / divisor,
		Bearish:    mean.Bearish / divisor,
		Choppiness: mean.Choppiness / divisor,
	}, nil
}

func extractRegimeMetrics(returns []float64, minSamples int) (regimeMetrics, error) {
	if len(returns) < minSamples {
		return regimeMetrics{}, errnie.Error(
			errors.New(
				"market: extract regime metrics: returns is less than min samples",
			),
		)
	}

	volatility := returnVolatility(returns)

	if volatility <= 0 {
		return regimeMetrics{}, errnie.Error(
			errors.New("market: extract regime metrics: volatility is less than or equal to 0"),
		)
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
	}, nil
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
	metrics, err := extractRegimeMetrics(returns, minSamples)

	if err != nil {
		return RegimeStrengths{}, errnie.Error(err)
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

func (classifier *RegimeClassifier) PublishFrame(bus *internal.Bus) error {
	if bus == nil {
		return errnie.Error(errors.New("market: publish frame bus is nil"))
	}

	mean, err := classifier.MarketMean()

	if err != nil {
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
