package market

import (
	"math"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/telemetry"
)

type AdaptationConfig struct {
	AlphaMin         float64
	AlphaMax         float64
	MinObs           int
	TrendSigma       float64
	StrongTrendSigma float64
	VolFloorSigma    float64
	VolScaleFloor    float64
	SeedVolScale     float64
	SlowWindowMax    int
	SlowWindowMin    int
	WindowVolFloor   float64
}

func (cfg *AdaptationConfig) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"alpha_min":          cfg.AlphaMin,
		"alpha_max":          cfg.AlphaMax,
		"min_obs":            cfg.MinObs,
		"trend_sigma":        cfg.TrendSigma,
		"strong_trend_sigma": cfg.StrongTrendSigma,
		"vol_floor_sigma":    cfg.VolFloorSigma,
		"vol_scale_floor":    cfg.VolScaleFloor,
		"slow_window_max":    cfg.SlowWindowMax,
		"slow_window_min":    cfg.SlowWindowMin,
	}))
}

func AdaptationConfigFromViper() (*AdaptationConfig, error) {
	return config.NewSafeConfig(&AdaptationConfig{
		AlphaMin:         viper.GetFloat64("regime.baseline.alpha_min"),
		AlphaMax:         viper.GetFloat64("regime.baseline.alpha_max"),
		MinObs:           viper.GetInt("regime.baseline.min_obs"),
		TrendSigma:       viper.GetFloat64("regime.baseline.trend_sigma"),
		StrongTrendSigma: viper.GetFloat64("regime.baseline.strong_trend_sigma"),
		VolFloorSigma:    viper.GetFloat64("regime.baseline.vol_floor_sigma"),
		VolScaleFloor:    viper.GetFloat64("regime.baseline.vol_scale_floor"),
		SeedVolScale:     viper.GetFloat64("regime.baseline.seed_vol_scale"),
		SlowWindowMax:    viper.GetInt("signals.causal.contagion_window_slow_max"),
		SlowWindowMin:    viper.GetInt("signals.causal.contagion_window_slow_min"),
		WindowVolFloor:   viper.GetFloat64("signals.causal.contagion_window_vol_floor"),
	})
}

/*
AdaptationController owns cross-section adaptive baselines and surprise-modulated
forgetting rates shared by regime scoring and causal window sizing.
*/
type AdaptationController struct {
	config        AdaptationConfig
	volScale      Baseline
	trendScore    Baseline
	windowAnchor  Baseline
	lastMedianVol float64
	seeded        bool
}

type adaptationState struct {
	controller *AdaptationController
	err        error
}

var adaptationValue atomic.Pointer[adaptationState]

func LoadAdaptation() (*AdaptationController, error) {
	if loaded := adaptationValue.Load(); loaded != nil {
		return loaded.controller, loaded.err
	}

	controller, err := NewAdaptationController()
	built := &adaptationState{controller: controller, err: err}

	if adaptationValue.CompareAndSwap(nil, built) {
		return controller, err
	}

	loaded := adaptationValue.Load()

	return loaded.controller, loaded.err
}

func NewAdaptationController() (*AdaptationController, error) {
	adaptationConfig, err := AdaptationConfigFromViper()

	if err != nil {
		return nil, errnie.Error(err)
	}

	if adaptationConfig.AlphaMax <= adaptationConfig.AlphaMin {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"regime.baseline.alpha_max": adaptationConfig.AlphaMax,
			"regime.baseline.alpha_min": adaptationConfig.AlphaMin,
		}))
	}

	if adaptationConfig.SlowWindowMax <= 0 {
		adaptationConfig.SlowWindowMax = 128
	}

	if adaptationConfig.SlowWindowMin <= 0 {
		adaptationConfig.SlowWindowMin = 16
	}

	if adaptationConfig.SlowWindowMin > adaptationConfig.SlowWindowMax {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"signals.causal.contagion_window_slow_min": adaptationConfig.SlowWindowMin,
			"signals.causal.contagion_window_slow_max": adaptationConfig.SlowWindowMax,
		}))
	}

	if adaptationConfig.VolScaleFloor <= 0 {
		adaptationConfig.VolScaleFloor = 1e-6
	}

	if adaptationConfig.SeedVolScale <= 0 {
		adaptationConfig.SeedVolScale = adaptationConfig.VolScaleFloor
	}

	if adaptationConfig.WindowVolFloor <= 0 {
		adaptationConfig.WindowVolFloor = adaptationConfig.VolScaleFloor
	}

	controller := &AdaptationController{
		config:       *adaptationConfig,
		volScale:     *NewBaseline(adaptationConfig.VolScaleFloor, adaptationConfig.MinObs),
		trendScore:   *NewBaseline(0.05, adaptationConfig.MinObs),
		windowAnchor: *NewBaseline(adaptationConfig.WindowVolFloor, adaptationConfig.MinObs),
	}

	controller.seed()

	return controller, nil
}

func (controller *AdaptationController) seed() {
	if controller.seeded {
		return
	}

	alpha := controller.config.AlphaMin

	_ = controller.volScale.Observe(controller.config.SeedVolScale, alpha)
	_ = controller.trendScore.Observe(1.25, alpha)
	_ = controller.windowAnchor.Observe(controller.config.SeedVolScale, alpha)
	controller.lastMedianVol = controller.config.SeedVolScale
	controller.seeded = true
}

func (controller *AdaptationController) Alpha() float64 {
	return AlphaFromSurprise(
		telemetry.MarketSurpriseIndex(),
		controller.config.AlphaMin,
		controller.config.AlphaMax,
	)
}

func (controller *AdaptationController) RegimeDynamics() RegimeDynamics {
	return RegimeDynamics{
		volScale:         &controller.volScale,
		trendScore:       &controller.trendScore,
		trendSigma:       controller.config.TrendSigma,
		strongTrendSigma: controller.config.StrongTrendSigma,
		volFloorSigma:    controller.config.VolFloorSigma,
		volScaleFloor:    controller.config.VolScaleFloor,
	}
}

func (controller *AdaptationController) ObserveRegimeSamples(
	volatilities []float64,
	trendScores []float64,
) {
	if len(volatilities) == 0 && len(trendScores) == 0 {
		return
	}

	alpha := controller.Alpha()

	if len(volatilities) > 0 {
		medianVol := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(volatilities...)...))
		controller.lastMedianVol = medianVol
		_ = controller.volScale.Observe(medianVol, alpha)
		_ = controller.windowAnchor.Observe(medianVol, controller.config.AlphaMin)
	}

	if len(trendScores) > 0 {
		_ = controller.trendScore.Observe(float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(trendScores...)...)), alpha)
	}
}

func (controller *AdaptationController) ContagionWindows() (fastWindow, mediumWindow, slowWindow int) {
	slowWindow = (controller.config.SlowWindowMin + controller.config.SlowWindowMax) / 2

	if !controller.windowAnchor.Ready() || controller.lastMedianVol <= 0 {
		return contagionDerivedWindows(slowWindow)
	}

	anchor := controller.windowAnchor.Scale()

	if anchor <= 0 {
		return contagionDerivedWindows(slowWindow)
	}

	ratio := controller.lastMedianVol / anchor

	if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return contagionDerivedWindows(slowWindow)
	}

	nominal := float64(controller.config.SlowWindowMin+controller.config.SlowWindowMax) / 2
	scaled := nominal * ratio
	slowWindow = int(math.Round(scaled))

	if slowWindow < controller.config.SlowWindowMin {
		slowWindow = controller.config.SlowWindowMin
	}

	if slowWindow > controller.config.SlowWindowMax {
		slowWindow = controller.config.SlowWindowMax
	}

	return contagionDerivedWindows(slowWindow)
}

func contagionDerivedWindows(slowWindow int) (fastWindow, mediumWindow, slowWindowOut int) {
	if slowWindow < 1 {
		slowWindow = 1
	}

	mediumWindow = slowWindow / 2

	if mediumWindow < 1 {
		mediumWindow = 1
	}

	fastWindow = slowWindow / 8

	if fastWindow < 1 {
		fastWindow = 1
	}

	return fastWindow, mediumWindow, slowWindow
}
