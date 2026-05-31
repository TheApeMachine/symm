package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultQuoteCurrency = "EUR"
	DefaultWalletEUR     = 200.0
	// DefaultTakerFeePct is the FALLBACK taker fee used only when a pair has no
	// real fee schedule from Kraken's AssetPairs endpoint. It is set to the true
	// bottom-tier (0-volume) taker rate that 1453 of 1547 Kraken pairs charge, so
	// an unmatched pair is still costed honestly rather than optimistically.
	DefaultTakerFeePct = 0.40
	DefaultSlippageBps = 0.0 // use live bid/ask half-spread; fallback only when quote missing
)

const (
	// Exploration defaults (explore-then-exploit). A cold (source, regime) bucket
	// has no demonstrated edge, so the disciplined edge gate would veto it forever
	// and the bucket would never gather the forward returns needed to learn its
	// edge. Exploration takes small, stop-protected, tagged paper positions on
	// cold buckets to gather that ground truth, then the bucket graduates to
	// edge-gated entries once it has enough samples.
	DefaultExplorationEnabled       = true
	DefaultExplorationPaperOnly     = true
	DefaultExplorationFraction      = 0.02 // probe size = 2% of available cash when Kelly is cold
	DefaultExplorationMinSamples    = 30   // explore until a bucket has this many settled samples
	DefaultExplorationMaxConcurrent = 3    // cap simultaneous exploratory positions
)

/*
Config holds runtime parameters for the trading engine.
*/
type Config struct {
	QuoteCurrency                string
	Symbols                      []string // watch list every signal subscribes to
	WalletEUR                    float64
	TakerFeePct                  float64 // fallback taker fee when a pair has no real schedule
	Fee30DVolume                 float64 // 30-day traded volume (fee_volume_currency) for fee-tier selection
	ExplorationEnabled           bool    // take small paper positions on cold buckets to learn their edge
	ExplorationPaperOnly         bool    // restrict exploration to paper wallets (negative-EV on real capital)
	ExplorationFraction          float64 // probe size as a fraction of available cash when Kelly is cold
	ExplorationMinSamples        int     // explore a bucket until it has this many settled samples
	ExplorationMaxConcurrent     int     // cap on simultaneous exploratory positions
	SlippageBPS                  float64
	BookDepthLevels              int
	SubscribeBatch               int
	MinQuoteCoverage             float64
	PriceHistory                 int
	MinCostEUR                   float64
	MinHoldBeforeRotate          time.Duration
	ScalpHoldBeforeExit          time.Duration
	FlowHoldBeforeExit           time.Duration
	EntryEdgeMultiple            float64       // Multiple of round-trip friction required before entry.
	TakeProfitR                  float64       // Minimum profit ratio versus stop distance, in R units.
	TakeProfitCapture            float64       // Fraction of predicted return at which a fixed profit target is armed.
	StopVolMultiple              float64       // Multiplier on recent per-tick volatility for stop distance.
	MinExhaustHold               time.Duration // Minimum hold before soft exhaust exits can close a position.
	AdverseSelectionBPS          float64       // Maker-fill adverse-selection penalty in basis points.
	PumpTrailPct                 float64       // Fast-pump trailing-stop retrace from peak.
	PumpSlowTrailPct             float64       // Slow-pump trailing-stop retrace from peak.
	PumpHardStopPct              float64       // Initial hard floor below entry for any pump.
	PumpSizeFraction             float64       // Size multiplier applied to pump-regime slots.
	PumpPullbackMin              float64       // Fast-pump: minimum retrace from peak to enter.
	PumpPullbackMax              float64       // Fast-pump: maximum retrace from peak to enter.
	TrailSpreadMultiple          float64
	DefaultTrailPct              float64
	MinTrailPct                  float64
	MaxTrailPct                  float64
	MaxLossPerTradeEUR           float64
	MaxDailyLossEUR              float64
	MaxPortfolioDrawdownPct      float64
	MaxDeployPct                 float64
	MaxSymbolCorrelation         float64
	MaxCorrelatedSlots           int
	MinCorrelationSamples        int
	CorrelationBarSeconds        int
	MaxEntrySlippageBPS          float64
	MaxSpreadBPS                 float64
	ExecutionMakerFallbackTicks  int
	AllowPaperShorts             bool
	AllowLiveShorts              bool
	KellyFraction                float64
	UseMakerEntries              bool
	MakerFeePct                  float64
	ForecastSpreadMultiple       float64
	ExitUrgencyThreshold         float64
	SnapshotFreshnessTTL         time.Duration
	MinCalibrationSamples        int
	MinConfidenceHistory         int
	ForwardReturnMinSamples      int
	PumpForwardReturnMinSamples  int
	ForwardReturnSignificanceZ   float64
	ForwardReturnSlopeAlpha      float64
	ExecutionEconomicsEnabled    bool          // label fills and gate entries on post-cost forward returns
	ExecutionStressEnabled       bool          // perturb quotes before paper fill (latency, depth, rejects)
	ExecutionStressLatency       time.Duration // additional quote age applied under stress
	ExecutionStressDepthFraction float64       // multiply visible depth qty (0–1]
	ExecutionStressRejectRate    float64       // Bernoulli reject before stressed fill
	ExecutionForwardWindow       time.Duration // horizon for forward-return labels after entry
	RegimeShockWindow            int
	RegimeShockMinSamples        int
	RegimeShockZScore            float64
	RegimeShockRecoverySamples   int
	RegimeShockTrustFloor        float64
	PerspectiveTTL               time.Duration
	NoiseFloorSNR                float64
	MaxPerspectiveMeasurements   int
	CalibrationHalfLifeFloor     time.Duration
	CalibrationHalfLifeCeiling   time.Duration
	CalibrationRunwayFactor      float64
	CalibrationShockSigma        float64
	CalibrationRecoveryFactor    float64
	CalibrationRecoveryBand      float64
	CalibrationRecoverySamples   int
	CalibrationBaselineAlpha     float64
	CausalConditionSwitch        float64
	CausalContagionBreak         float64
	CausalContagionMinSamples    int
	CausalContagionWindow        int
	TrailRiskEMAAlpha            float64
	TrailSpectralWidenAt         float64
	TrailSpectralWidenGain       float64
	TrailTurbWidenAt             float64
	TrailTurbWidenMultiple       float64
	TrailReynoldsWidenAt         float64
	TrailReynoldsWidenGain       float64
	TrailRiskDebounce            time.Duration
	BookDepthDecayLambda         float64
	SpoofWeightedThreshold       float64
	SpoofLevel1Reject            float64
	MinFillToCancelRatio         float64
	BookFluxWindow               time.Duration
	VolumeClockBarsPerDay        float64
	FractionalDiffOrder          float64
	FractionalDiffWidth          int
	FastPumpWindow               time.Duration
	MediumPumpWindow             time.Duration
	FastPumpVolumeRatio          float64
	SlowRVOLThreshold            float64
	SlowRVOLIntervalMinutes      int
	ExitPeakUrgency              float64
	HawkesFitCooldown            time.Duration
	CandleSeconds                int
	FluidGridSize                int
	FluidHeightEMAAlpha          float64
	FluidQuantileClip            float64
	ExitEvery                    time.Duration
	WSPingInterval               time.Duration
	UITelemetryBuffer            int
	UIHeartbeatInterval          time.Duration
	UIAddr                       string
	MaxPendingPerSignal          int
	MaxPendingGlobal             int
	WinBoostHalfLife             time.Duration
	MaxScanSymbols               int
	SymbolActivityHalfLife       time.Duration
	LogDir                       string
	PaperOrderLatency            time.Duration
	PaperMinFillCoverage         float64
	PaperOrderRejectRate         float64
	LiveInventoryEpsilon         float64
	LogLevel                     string
	LogFileActive                bool
	LogStdoutActive              bool
	LiveTradingEnabled           bool
	KrakenAPIKey                 string
	KrakenAPISecret              string
	OHLCEWarmEnabled             bool
	OHLCIntervalMinutes          int
	OHLCMaxSymbols               int
	OHLCEWarmPulseCredit         int
	ReplayFile                   string
	ReplayLoop                   bool
	ReplayPace                   time.Duration
	RecordFile                   string
	AuditFile                    string
	AuditMaxFileBytes            int64
	AuditMaxFiles                int
	AuditGateRejectCooldown      time.Duration
	ConfigFile                   string
	PerspectiveFile              string
	Headless                     bool
}

var System *Config

const defaultTunedFile = "runs/tuned.json"
const defaultPerspectiveFile = "config/perspectives.yaml"

func init() {
	if err := Bootstrap(); err != nil {
		panic(err)
	}
}

/*
Bootstrap loads defaults, optional tunables, and builds Runtime. Invalid config
or tunables files fail closed instead of silently continuing with defaults.
*/
func Bootstrap() error {
	System = NewConfig()

	if path := strings.TrimSpace(System.ConfigFile); path != "" {
		if err := LoadTunablesFile(path, System); err != nil {
			return fmt.Errorf("load config file %q: %w", path, err)
		}

		Runtime = NewRuntime(System)
		syncPerspectives(System)

		return nil
	}

	if _, err := os.Stat(defaultTunedFile); err == nil {
		if err := LoadTunablesFile(defaultTunedFile, System); err != nil {
			return fmt.Errorf("load tuned config %q: %w", defaultTunedFile, err)
		}
	}

	Runtime = NewRuntime(System)
	syncPerspectives(System)

	return nil
}

/*
DefaultSymbols is the watch list every signal subscribes to: a set of liquid
EUR spot pairs with BTC/EUR as the dashboard anchor.
*/
func DefaultSymbols() []string {
	return []string{
		"BTC/EUR",
		"ETH/EUR",
		"SOL/EUR",
		"XRP/EUR",
		"ADA/EUR",
		"DOGE/EUR",
		"DOT/EUR",
		"LINK/EUR",
		"AVAX/EUR",
		"LTC/EUR",
	}
}

/*
NewConfig returns paper-trading defaults for the €200 challenge.
*/
func NewConfig() *Config {
	cfg := &Config{
		QuoteCurrency:                DefaultQuoteCurrency,
		Symbols:                      DefaultSymbols(),
		WalletEUR:                    DefaultWalletEUR,
		TakerFeePct:                  DefaultTakerFeePct,
		Fee30DVolume:                 0, // small/paper account sits at the bottom (highest) fee tier
		ExplorationEnabled:           DefaultExplorationEnabled,
		ExplorationPaperOnly:         DefaultExplorationPaperOnly,
		ExplorationFraction:          DefaultExplorationFraction,
		ExplorationMinSamples:        DefaultExplorationMinSamples,
		ExplorationMaxConcurrent:     DefaultExplorationMaxConcurrent,
		SlippageBPS:                  DefaultSlippageBps,
		BookDepthLevels:              5,
		ExitEvery:                    10 * time.Millisecond,
		SubscribeBatch:               50,
		MinQuoteCoverage:             0.95,
		PriceHistory:                 128,
		MinCostEUR:                   0.45,
		MinHoldBeforeRotate:          time.Minute,
		ScalpHoldBeforeExit:          90 * time.Second,
		FlowHoldBeforeExit:           30 * time.Second,
		EntryEdgeMultiple:            2.0,             // Require forecast >= 2x round-trip friction.
		TakeProfitR:                  2.0,             // Require forecast >= 2R relative to stop distance.
		TakeProfitCapture:            0.75,            // Exit at 75% of the calibrated expected return.
		StopVolMultiple:              8.0,             // Stop distance = 8x recent per-tick volatility, bounded.
		MinExhaustHold:               5 * time.Second, // Suppress soft exits for first five seconds.
		AdverseSelectionBPS:          5.0,             // Add 5 bps to filled maker paper entry cost.
		PumpTrailPct:                 0.08,            // Fast-pump trailing stop: 8% retrace from peak.
		PumpSlowTrailPct:             0.20,            // Slow-pump trailing stop: 20% retrace from peak.
		PumpHardStopPct:              0.12,            // Hard floor 12% below pump entry.
		PumpSizeFraction:             0.25,            // Pump slots sized at 25% of the normal slot.
		PumpPullbackMin:              0.03,            // Fast-pump entry: require >=3% retrace from peak.
		PumpPullbackMax:              0.20,            // Fast-pump entry: skip if >20% retrace (leg is dead).
		TrailSpreadMultiple:          2,
		DefaultTrailPct:              0.35,
		MinTrailPct:                  0.15,
		MaxTrailPct:                  3.0,
		MaxLossPerTradeEUR:           2,
		MaxDailyLossEUR:              20,
		MaxSymbolCorrelation:         0.85,
		MaxCorrelatedSlots:           1,
		MinCorrelationSamples:        12,
		CorrelationBarSeconds:        10,
		MaxEntrySlippageBPS:          50,
		MaxSpreadBPS:                 0,
		ExecutionMakerFallbackTicks:  4,
		AllowPaperShorts:             false,
		AllowLiveShorts:              false,
		KellyFraction:                0.5,
		UseMakerEntries:              true,
		MakerFeePct:                  0.25, // fallback maker fee: real bottom-tier (0-volume) rate
		ForecastSpreadMultiple:       4,
		ExitUrgencyThreshold:         0.65,
		SnapshotFreshnessTTL:         200 * time.Millisecond,
		MinCalibrationSamples:        12,
		MinConfidenceHistory:         4,
		ForwardReturnMinSamples:      30,
		PumpForwardReturnMinSamples:  8,
		ForwardReturnSignificanceZ:   1.96,
		ForwardReturnSlopeAlpha:      0.05,
		ExecutionEconomicsEnabled:    true,
		ExecutionStressEnabled:       false,
		ExecutionStressLatency:       75 * time.Millisecond,
		ExecutionStressDepthFraction: 0.35,
		ExecutionStressRejectRate:    0.05,
		ExecutionForwardWindow:       30 * time.Second,
		RegimeShockWindow:            128,
		RegimeShockMinSamples:        64,
		RegimeShockZScore:            6,
		RegimeShockRecoverySamples:   64,
		RegimeShockTrustFloor:        0.02,
		PerspectiveTTL:               30 * time.Second,
		NoiseFloorSNR:                1.0,
		MaxPerspectiveMeasurements:   256,
		CalibrationHalfLifeFloor:     2 * time.Second,
		CalibrationHalfLifeCeiling:   15 * time.Minute,
		CalibrationRunwayFactor:      0.5,
		CalibrationShockSigma:        3,
		CalibrationRecoveryFactor:    6,
		CalibrationRecoveryBand:      0.1,
		CalibrationRecoverySamples:   3,
		CalibrationBaselineAlpha:     0.05,
		CausalConditionSwitch:        1000,
		CausalContagionBreak:         0.9,
		CausalContagionMinSamples:    16,
		CausalContagionWindow:        128,
		TrailRiskEMAAlpha:            0.2,
		TrailSpectralWidenAt:         0.85,
		TrailSpectralWidenGain:       4,
		TrailTurbWidenAt:             1,
		TrailTurbWidenMultiple:       1.5,
		TrailReynoldsWidenAt:         50,
		TrailReynoldsWidenGain:       0.01,
		TrailRiskDebounce:            500 * time.Millisecond,
		BookDepthDecayLambda:         1000,
		SpoofWeightedThreshold:       0.5,
		SpoofLevel1Reject:            -0.1,
		MinFillToCancelRatio:         0.15,
		BookFluxWindow:               10 * time.Second,
		VolumeClockBarsPerDay:        8640,
		FractionalDiffOrder:          0.4,
		FractionalDiffWidth:          16,
		FastPumpWindow:               10 * time.Second,
		MediumPumpWindow:             5 * time.Minute,
		FastPumpVolumeRatio:          15,
		SlowRVOLThreshold:            5,
		SlowRVOLIntervalMinutes:      60,
		ExitPeakUrgency:              0.8,
		HawkesFitCooldown:            5 * time.Second,
		CandleSeconds:                5,
		FluidGridSize:                32,
		FluidHeightEMAAlpha:          0.35,
		FluidQuantileClip:            0.95,
		WSPingInterval:               30 * time.Second,
		UITelemetryBuffer:            512,
		UIHeartbeatInterval:          250 * time.Millisecond,
		UIAddr:                       ":8765",
		MaxPendingPerSignal:          4096,
		MaxPendingGlobal:             0,
		WinBoostHalfLife:             2 * time.Hour,
		MaxScanSymbols:               64,
		SymbolActivityHalfLife:       30 * time.Second,
		LogDir:                       "runs",
		PaperOrderLatency:            0,
		PaperMinFillCoverage:         1,
		PaperOrderRejectRate:         0,
		LiveInventoryEpsilon:         1e-8,
		LogLevel:                     "info",
		LogFileActive:                true,
		LogStdoutActive:              false,
		OHLCEWarmEnabled:             true,
		OHLCIntervalMinutes:          5,
		OHLCMaxSymbols:               64,
		OHLCEWarmPulseCredit:         30,
		AuditMaxFileBytes:            32 << 20,
		AuditMaxFiles:                3,
		AuditGateRejectCooldown:      60 * time.Second,
		PerspectiveFile:              defaultPerspectiveFile,
	}

	if cfg.MaxPortfolioDrawdownPct <= 0 && cfg.WalletEUR > 0 {
		cfg.MaxPortfolioDrawdownPct = cfg.MaxDailyLossEUR / cfg.WalletEUR
	}

	if err := ApplyEnvironment(cfg); err != nil {
		panic(err)
	}

	return cfg
}

func ApplyEnvironment(cfg *Config) error {
	if replayFile := strings.TrimSpace(os.Getenv("SYMM_REPLAY_FILE")); replayFile != "" {
		cfg.ReplayFile = replayFile
	}

	if replayLoop := strings.TrimSpace(os.Getenv("SYMM_REPLAY_LOOP")); replayLoop == "1" ||
		strings.EqualFold(replayLoop, "true") {
		cfg.ReplayLoop = true
	}

	if pace := strings.TrimSpace(os.Getenv("SYMM_REPLAY_PACE")); pace != "" {
		parsed, err := time.ParseDuration(pace)

		if err != nil {
			return fmt.Errorf("SYMM_REPLAY_PACE: %w", err)
		}

		cfg.ReplayPace = parsed
	}

	if recordFile := strings.TrimSpace(os.Getenv("SYMM_RECORD_FILE")); recordFile != "" {
		cfg.RecordFile = recordFile
	}

	if auditFile := strings.TrimSpace(os.Getenv("SYMM_AUDIT_FILE")); auditFile != "" {
		cfg.AuditFile = auditFile
	}

	if auditCooldown := strings.TrimSpace(os.Getenv("SYMM_AUDIT_GATE_COOLDOWN")); auditCooldown != "" {
		parsed, err := time.ParseDuration(auditCooldown)

		if err != nil {
			return fmt.Errorf("SYMM_AUDIT_GATE_COOLDOWN: %w", err)
		}

		cfg.AuditGateRejectCooldown = parsed
	}

	if auditMaxMB := strings.TrimSpace(os.Getenv("SYMM_AUDIT_MAX_MB")); auditMaxMB != "" {
		var megabytes int64

		if _, err := fmt.Sscan(auditMaxMB, &megabytes); err != nil {
			return fmt.Errorf("SYMM_AUDIT_MAX_MB: %w", err)
		}

		if megabytes <= 0 {
			return fmt.Errorf("SYMM_AUDIT_MAX_MB: must be positive")
		}

		cfg.AuditMaxFileBytes = megabytes << 20
	}

	if configFile := strings.TrimSpace(os.Getenv("SYMM_CONFIG_FILE")); configFile != "" {
		cfg.ConfigFile = configFile
	}

	if perspectiveFile := strings.TrimSpace(os.Getenv("SYMM_PERSPECTIVES_FILE")); perspectiveFile != "" {
		cfg.PerspectiveFile = perspectiveFile
	}

	if headless := strings.TrimSpace(os.Getenv("SYMM_HEADLESS")); headless == "1" ||
		strings.EqualFold(headless, "true") {
		cfg.Headless = true
	}

	cfg.KrakenAPIKey = strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_KEY"))
	cfg.KrakenAPISecret = strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_SECRET"))

	if live := strings.TrimSpace(os.Getenv("SYMM_LIVE")); live == "1" ||
		strings.EqualFold(live, "true") {
		cfg.LiveTradingEnabled = true
	}

	if stdout := strings.TrimSpace(os.Getenv("SYMM_LOG_STDOUT")); stdout == "1" ||
		strings.EqualFold(stdout, "true") {
		cfg.LogStdoutActive = true
	}

	if stress := strings.TrimSpace(os.Getenv("SYMM_EXECUTION_STRESS")); stress == "1" ||
		strings.EqualFold(stress, "true") {
		cfg.ExecutionStressEnabled = true
	}

	return nil
}
