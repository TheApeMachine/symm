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
	QuoteCurrency          string
	WalletEUR              float64
	RescoreEvery           time.Duration
	TakerFeePct            float64
	SlippageBPS            float64
	SubscribeBatch         int
	MinWarmPulses          int
	MinQuoteCoverage       float64
	PriceHistory           int
	MinCostEUR             float64
	MaxSlotPct             float64
	MaxSlots               int
	MinHoldBeforeRotate    time.Duration
	ScalpHoldBeforeExit    time.Duration
	FlowHoldBeforeExit     time.Duration
	TrailSpreadMultiple    float64
	ExitEvery              time.Duration
	WSPingInterval         time.Duration
	UIAddr                 string
	MaxPendingPerSignal    int
	MaxPendingGlobal       int
	WinBoostHalfLife       time.Duration
	MaxScanSymbols         int
	SymbolActivityHalfLife time.Duration
	LogDir                 string
	LogLevel               string
	LogFileActive          bool
	ReplayFile             string
	ReplayPace             time.Duration
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
		QuoteCurrency:          DefaultQuoteCurrency,
		WalletEUR:              DefaultWalletEUR,
		TakerFeePct:            DefaultTakerFeePct,
		SlippageBPS:            DefaultSlippageBps,
		ExitEvery:              10 * time.Millisecond,
		RescoreEvery:           100 * time.Millisecond,
		SubscribeBatch:         50,
		MinWarmPulses:          50,
		MinQuoteCoverage:       0.95,
		PriceHistory:           128,
		MinCostEUR:             0.45,
		MaxSlotPct:             5,
		MaxSlots:               4,
		MinHoldBeforeRotate:    time.Minute,
		ScalpHoldBeforeExit:    15 * time.Second,
		FlowHoldBeforeExit:     30 * time.Second,
		TrailSpreadMultiple:    2,
		WSPingInterval:         30 * time.Second,
		UIAddr:                 ":8765",
		MaxPendingPerSignal:    4096,
		MaxPendingGlobal:       0,
		WinBoostHalfLife:       2 * time.Hour,
		MaxScanSymbols:         64,
		SymbolActivityHalfLife: 30 * time.Second,
		LogDir:                 "runs",
		LogLevel:               "info",
		LogFileActive:          true,
		ReplayPace:             50 * time.Millisecond,
	}

	if replayFile := strings.TrimSpace(os.Getenv("SYMM_REPLAY_FILE")); replayFile != "" {
		cfg.ReplayFile = replayFile
	}

	if replayPace := strings.TrimSpace(os.Getenv("SYMM_REPLAY_PACE")); replayPace != "" {
		if parsed, err := time.ParseDuration(replayPace); err == nil {
			cfg.ReplayPace = parsed
		}
	}

	return cfg
}

/*
TakerFee models Kraken-style taker fee on notional (percent).
*/
func (cfg Config) TakerFee(notionalEUR, feePct float64) float64 {
	if notionalEUR <= 0 || feePct <= 0 {
		return 0
	}

	return notionalEUR * feePct / 100
}

/*
SlippagePrice applies half-spread plus extra bps on exit (worse fill).
*/
func (cfg Config) SlippagePrice(
	last, bid, ask float64,
	side string,
	extraBPS float64,
) float64 {
	if last <= 0 {
		return last
	}

	halfSpread := 0.0

	if bid > 0 && ask > 0 && ask >= bid {
		halfSpread = (ask - bid) / 2
	}

	extra := last * extraBPS / 10000

	switch side {
	case "buy":
		return last + halfSpread + extra
	case "sell":
		return last - halfSpread - extra
	default:
		return last
	}
}
