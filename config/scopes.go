package config

import "time"

/*
ExecutionScope holds order-routing and fill-simulation parameters isolated from
the monolithic Config so the trader and broker can inject per-process copies.
*/
type ExecutionScope struct {
	QuoteCurrency               string
	SlippageBPS                 float64
	SnapshotFreshnessTTL        time.Duration
	MaxSpreadBPS                float64
	MaxEntrySlippageBPS         float64
	AdverseSelectionBPS         float64
	UseMakerEntries             bool
	MakerFeePct                 float64
	ExecutionMakerFallbackTicks int
	PaperOrderRejectRate        float64
	ExecutionStressEnabled      bool
	ExecutionStressRejectRate   float64
	ExecutionEconomicsEnabled   bool
	ExecutionForwardWindow      time.Duration
	LiveInventoryEpsilon        float64
}

/*
SignalScope holds measurement and playbook calibration parameters.
*/
type SignalScope struct {
	Symbols                    []string
	HawkesFitCooldown          time.Duration
	PerspectiveTTL             time.Duration
	MaxPerspectiveMeasurements int
	MinCalibrationSamples      int
	RegimeShockWindow          int
	RegimeShockMinSamples      int
	RegimeShockZScore          float64
	BookDepthLevels            int
	SubscribeBatch             int
}

/*
UIScope holds dashboard and websocket telemetry parameters.
*/
type UIScope struct {
	UITelemetryBuffer   int
	UIHeartbeatInterval time.Duration
	UIAddr              string
	WSPingInterval      time.Duration
}

/*
RiskScope holds portfolio sizing and loss limits.
*/
type RiskScope struct {
	MinCostEUR               float64
	EntryEdgeMultiple        float64
	KellyFraction            float64
	MaxLossPerTradeEUR       float64
	MaxDailyLossEUR          float64
	MaxPortfolioDrawdownPct  float64
	MaxDeployPct             float64
	MinExhaustHold           time.Duration
	ExplorationEnabled       bool
	ExplorationPaperOnly     bool
	ExplorationFraction      float64
	ExplorationMinSamples    int
	ExplorationMaxConcurrent int
}

/*
ExecutionScopeFrom copies execution fields from cfg.
*/
func ExecutionScopeFrom(cfg *Config) ExecutionScope {
	if cfg == nil {
		return ExecutionScope{}
	}

	return ExecutionScope{
		QuoteCurrency:               cfg.QuoteCurrency,
		SlippageBPS:                 cfg.SlippageBPS,
		SnapshotFreshnessTTL:        cfg.SnapshotFreshnessTTL,
		MaxSpreadBPS:                cfg.MaxSpreadBPS,
		MaxEntrySlippageBPS:         cfg.MaxEntrySlippageBPS,
		AdverseSelectionBPS:         cfg.AdverseSelectionBPS,
		UseMakerEntries:             cfg.UseMakerEntries,
		MakerFeePct:                 cfg.MakerFeePct,
		ExecutionMakerFallbackTicks: cfg.ExecutionMakerFallbackTicks,
		PaperOrderRejectRate:        cfg.PaperOrderRejectRate,
		ExecutionStressEnabled:      cfg.ExecutionStressEnabled,
		ExecutionStressRejectRate:   cfg.ExecutionStressRejectRate,
		ExecutionEconomicsEnabled:   cfg.ExecutionEconomicsEnabled,
		ExecutionForwardWindow:      cfg.ExecutionForwardWindow,
		LiveInventoryEpsilon:        cfg.LiveInventoryEpsilon,
	}
}

/*
SignalScopeFrom copies signal fields from cfg.
*/
func SignalScopeFrom(cfg *Config) SignalScope {
	if cfg == nil {
		return SignalScope{}
	}

	symbols := make([]string, len(cfg.Symbols))
	copy(symbols, cfg.Symbols)

	return SignalScope{
		Symbols:                    symbols,
		HawkesFitCooldown:          cfg.HawkesFitCooldown,
		PerspectiveTTL:             cfg.PerspectiveTTL,
		MaxPerspectiveMeasurements: cfg.MaxPerspectiveMeasurements,
		MinCalibrationSamples:      cfg.MinCalibrationSamples,
		RegimeShockWindow:          cfg.RegimeShockWindow,
		RegimeShockMinSamples:      cfg.RegimeShockMinSamples,
		RegimeShockZScore:          cfg.RegimeShockZScore,
		BookDepthLevels:            cfg.BookDepthLevels,
		SubscribeBatch:             cfg.SubscribeBatch,
	}
}

/*
UIScopeFrom copies UI fields from cfg.
*/
func UIScopeFrom(cfg *Config) UIScope {
	if cfg == nil {
		return UIScope{}
	}

	return UIScope{
		UITelemetryBuffer:   cfg.UITelemetryBuffer,
		UIHeartbeatInterval: cfg.UIHeartbeatInterval,
		UIAddr:              cfg.UIAddr,
		WSPingInterval:      cfg.WSPingInterval,
	}
}

/*
RiskScopeFrom copies risk fields from cfg.
*/
func RiskScopeFrom(cfg *Config) RiskScope {
	if cfg == nil {
		return RiskScope{}
	}

	return RiskScope{
		MinCostEUR:               cfg.MinCostEUR,
		EntryEdgeMultiple:        cfg.EntryEdgeMultiple,
		KellyFraction:            cfg.KellyFraction,
		MaxLossPerTradeEUR:       cfg.MaxLossPerTradeEUR,
		MaxDailyLossEUR:          cfg.MaxDailyLossEUR,
		MaxPortfolioDrawdownPct:  cfg.MaxPortfolioDrawdownPct,
		MaxDeployPct:             cfg.MaxDeployPct,
		MinExhaustHold:           cfg.MinExhaustHold,
		ExplorationEnabled:       cfg.ExplorationEnabled,
		ExplorationPaperOnly:     cfg.ExplorationPaperOnly,
		ExplorationFraction:      cfg.ExplorationFraction,
		ExplorationMinSamples:    cfg.ExplorationMinSamples,
		ExplorationMaxConcurrent: cfg.ExplorationMaxConcurrent,
	}
}
