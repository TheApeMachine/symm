package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

/*
TradingConfig is the validated runtime trading envelope loaded once at boot.
*/
type TradingConfig struct {
	Model                  string
	PositionFraction       float64
	MaxConcurrentPositions int
	MaxQuoteAge            time.Duration
	MaxSpreadBps           float64
	MaxSlippageBps         float64
	OrderAckTimeout        time.Duration
	EntryTransitTTL        time.Duration
	AuditFile              string
}

func LoadTradingConfig() (TradingConfig, error) {
	config := TradingConfig{
		Model:                  viper.GetString("trading.model"),
		PositionFraction:       viper.GetFloat64("trading.position_fraction"),
		MaxConcurrentPositions: viper.GetInt("trading.max_concurrent_positions"),
		MaxQuoteAge:            viper.GetDuration("trading.max_quote_age"),
		MaxSpreadBps:           viper.GetFloat64("trading.max_spread_bps"),
		MaxSlippageBps:         viper.GetFloat64("trading.max_slippage_bps"),
		OrderAckTimeout:        viper.GetDuration("trading.order_ack_timeout"),
		EntryTransitTTL:        viper.GetDuration("trading.entry.transit_ttl"),
		AuditFile:              viper.GetString("trading.audit.file"),
	}

	return NewSafeConfig(config)
}

func (config TradingConfig) Validate() error {
	switch config.Model {
	case "paper", "live":
	default:
		return fmt.Errorf("config: trading.model must be paper or live, got %q", config.Model)
	}

	if config.PositionFraction <= 0 {
		return fmt.Errorf("config: trading.position_fraction must be positive")
	}

	if config.MaxConcurrentPositions <= 0 {
		return fmt.Errorf("config: trading.max_concurrent_positions must be positive")
	}

	if config.MaxQuoteAge <= 0 {
		return fmt.Errorf("config: trading.max_quote_age must be positive")
	}

	if config.MaxSpreadBps <= 0 {
		return fmt.Errorf("config: trading.max_spread_bps must be positive")
	}

	if config.MaxSlippageBps <= 0 {
		return fmt.Errorf("config: trading.max_slippage_bps must be positive")
	}

	if config.OrderAckTimeout <= 0 {
		return fmt.Errorf("config: trading.order_ack_timeout must be positive")
	}

	if config.EntryTransitTTL <= 0 {
		return fmt.Errorf("config: trading.entry.transit_ttl must be positive")
	}

	return nil
}
