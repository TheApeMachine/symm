package config

import (
	"os"
	"strings"
	"time"
)

const (
	DefaultQuoteCurrency = "EUR"
	DefaultWalletEUR     = 200.0
	DefaultTakerFeePct   = 0.26
	DefaultSlippageBps   = 0.0 // use live bid/ask half-spread; fallback only when quote missing
)

/*
Config holds runtime parameters for the trading engine.
*/
type Config struct {
	QuoteCurrency              string
	WalletEUR                  float64
	TakerFeePct                float64
	SlippageBPS                float64
	BookDepthLevels            int
	SubscribeBatch             int
	MinQuoteCoverage           float64
	PriceHistory               int
	MinCostEUR                 float64
	MaxSlotPct                 float64
	MinHoldBeforeRotate        time.Duration
	ScalpHoldBeforeExit        time.Duration
	FlowHoldBeforeExit         time.Duration
	EntryEdgeMultiple          float64       // Multiple of round-trip friction required before entry.
	TakeProfitR                float64       // Minimum profit ratio versus stop distance, in R units.
	StopVolMultiple            float64       // Multiplier on recent per-tick volatility for stop distance.
	MinExhaustHold             time.Duration // Minimum hold before soft exhaust exits can close a position.
	AdverseSelectionBPS        float64       // Maker-fill adverse-selection penalty in basis points.
	TrailSpreadMultiple        float64
	DefaultTrailPct            float64
	MinTrailPct                float64
	MaxTrailPct                float64
	MaxLossPerTradeEUR         float64
	MaxDailyLossEUR            float64
	MaxPortfolioDrawdownPct    float64
	MaxDeployPct               float64
	MaxSymbolCorrelation       float64
	MaxCorrelatedSlots         int
	MinCorrelationSamples      int
	CorrelationBarSeconds      int
	MaxEntrySlippageBPS        float64
	MaxSpreadBPS               float64
	AllowPaperShorts           bool
	AllowLiveShorts            bool
	KellyFraction              float64
	UseMakerEntries            bool
	MakerFeePct                float64
	ForecastSpreadMultiple     float64
	ExitUrgencyThreshold       float64
	MaxActivePerspectives      int
	SnapshotFreshnessTTL       time.Duration
	MinCalibrationSamples      int
	MinConfidenceHistory       int
	CalibrationHalfLifeFloor   time.Duration
	CalibrationHalfLifeCeiling time.Duration
	CalibrationRunwayFactor    float64
	TrailRiskEMAAlpha          float64
	TrailSpectralWidenAt       float64
	TrailSpectralWidenGain     float64
	TrailTurbWidenAt           float64
	TrailTurbWidenMultiple     float64
	TrailReynoldsWidenAt       float64
	TrailReynoldsWidenGain     float64
	TrailRiskDebounce          time.Duration
	BookDepthDecayLambda       float64
	SpoofWeightedThreshold     float64
	SpoofLevel1Reject          float64
	MinFillToCancelRatio       float64
	BookFluxWindow             time.Duration
	FastPumpWindow             time.Duration
	MediumPumpWindow           time.Duration
	FastPumpVolumeRatio        float64
	SlowRVOLThreshold          float64
	SlowRVOLIntervalMinutes    int
	ExitPeakUrgency            float64
	HawkesFitCooldown          time.Duration
	CandleSeconds              int
	FluidGridSize              int
	FluidHeightEMAAlpha        float64
	FluidQuantileClip          float64
	ExitEvery                  time.Duration
	WSPingInterval             time.Duration
	UIAddr                     string
	MaxPendingPerSignal        int
	MaxPendingGlobal           int
	WinBoostHalfLife           time.Duration
	MaxScanSymbols             int
	SymbolActivityHalfLife     time.Duration
	LogDir                     string
	PaperOrderLatency          time.Duration
	PaperMinFillCoverage       float64
	PaperOrderRejectRate       float64
	LiveInventoryEpsilon       float64
	LogLevel                   string
	LogFileActive              bool
	LogStdoutActive            bool
	KrakenAPIKey               string
	KrakenAPISecret            string
	OHLCEWarmEnabled           bool
	OHLCIntervalMinutes        int
	OHLCMaxSymbols             int
	OHLCEWarmPulseCredit       int
	ReplayFile                 string
}

var System *Config

func init() {
	System = NewConfig()
}

/*
NewConfig returns paper-trading defaults for the €200 challenge.
*/
func NewConfig() *Config {
	cfg := &Config{
		QuoteCurrency:              DefaultQuoteCurrency,
		WalletEUR:                  DefaultWalletEUR,
		TakerFeePct:                DefaultTakerFeePct,
		SlippageBPS:                DefaultSlippageBps,
		BookDepthLevels:            5,
		ExitEvery:                  10 * time.Millisecond,
		SubscribeBatch:             50,
		MinQuoteCoverage:           0.95,
		PriceHistory:               128,
		MinCostEUR:                 0.45,
		MaxSlotPct:                 5,
		MinHoldBeforeRotate:        time.Minute,
		ScalpHoldBeforeExit:        90 * time.Second,
		FlowHoldBeforeExit:         30 * time.Second,
		EntryEdgeMultiple:          2.0,             // Require forecast >= 2x round-trip friction.
		TakeProfitR:                2.0,             // Require forecast >= 2R relative to stop distance.
		StopVolMultiple:            8.0,             // Stop distance = 8x recent per-tick volatility, bounded.
		MinExhaustHold:             5 * time.Second, // Suppress soft exits for first five seconds.
		AdverseSelectionBPS:        5.0,             // Add 5 bps to filled maker paper entry cost.
		TrailSpreadMultiple:        2,
		DefaultTrailPct:            0.35,
		MinTrailPct:                0.15,
		MaxTrailPct:                3.0,
		MaxLossPerTradeEUR:         2,
		MaxDailyLossEUR:            20,
		MaxSymbolCorrelation:       0.85,
		MaxCorrelatedSlots:         1,
		MinCorrelationSamples:      12,
		CorrelationBarSeconds:      10,
		MaxEntrySlippageBPS:        50,
		MaxSpreadBPS:               0,
		AllowPaperShorts:           false,
		AllowLiveShorts:            false,
		KellyFraction:              0.5,
		UseMakerEntries:            false,
		MakerFeePct:                0.16,
		ForecastSpreadMultiple:     4,
		ExitUrgencyThreshold:       0.65,
		MaxActivePerspectives:      2,
		SnapshotFreshnessTTL:       200 * time.Millisecond,
		MinCalibrationSamples:      12,
		MinConfidenceHistory:       4,
		CalibrationHalfLifeFloor:   2 * time.Second,
		CalibrationHalfLifeCeiling: 15 * time.Minute,
		CalibrationRunwayFactor:    0.5,
		TrailRiskEMAAlpha:          0.2,
		TrailSpectralWidenAt:       0.85,
		TrailSpectralWidenGain:     4,
		TrailTurbWidenAt:           1,
		TrailTurbWidenMultiple:     1.5,
		TrailReynoldsWidenAt:       50,
		TrailReynoldsWidenGain:     0.01,
		TrailRiskDebounce:          500 * time.Millisecond,
		BookDepthDecayLambda:       1000,
		SpoofWeightedThreshold:     0.5,
		SpoofLevel1Reject:          -0.1,
		MinFillToCancelRatio:       0.15,
		BookFluxWindow:             10 * time.Second,
		FastPumpWindow:             10 * time.Second,
		MediumPumpWindow:           5 * time.Minute,
		FastPumpVolumeRatio:        15,
		SlowRVOLThreshold:          5,
		SlowRVOLIntervalMinutes:    60,
		ExitPeakUrgency:            0.8,
		HawkesFitCooldown:          5 * time.Second,
		CandleSeconds:              5,
		FluidGridSize:              32,
		FluidHeightEMAAlpha:        0.35,
		FluidQuantileClip:          0.95,
		WSPingInterval:             30 * time.Second,
		UIAddr:                     ":8765",
		MaxPendingPerSignal:        4096,
		MaxPendingGlobal:           0,
		WinBoostHalfLife:           2 * time.Hour,
		MaxScanSymbols:             64,
		SymbolActivityHalfLife:     30 * time.Second,
		LogDir:                     "runs",
		PaperOrderLatency:          0,
		PaperMinFillCoverage:       1,
		PaperOrderRejectRate:       0,
		LiveInventoryEpsilon:       1e-8,
		LogLevel:                   "info",
		LogFileActive:              true,
		LogStdoutActive:            false,
		OHLCEWarmEnabled:           true,
		OHLCIntervalMinutes:        5,
		OHLCMaxSymbols:             64,
		OHLCEWarmPulseCredit:       30,
	}

	if cfg.MaxPortfolioDrawdownPct <= 0 && cfg.WalletEUR > 0 {
		cfg.MaxPortfolioDrawdownPct = cfg.MaxDailyLossEUR / cfg.WalletEUR
	}

	if replayFile := strings.TrimSpace(os.Getenv("SYMM_REPLAY_FILE")); replayFile != "" {
		cfg.ReplayFile = replayFile
	}

	cfg.KrakenAPIKey = strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_KEY"))
	cfg.KrakenAPISecret = strings.TrimSpace(os.Getenv("SYMM_KRAKEN_API_SECRET"))

	if stdout := strings.TrimSpace(os.Getenv("SYMM_LOG_STDOUT")); stdout == "1" ||
		strings.EqualFold(stdout, "true") {
		cfg.LogStdoutActive = true
	}

	return cfg
}
