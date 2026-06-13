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
	MaxConcurrentPositions int
	OpportunitySlotCount   int
	MaxQuoteAge            time.Duration
	OrderAckTimeout        time.Duration
	EntryTransitTTL        time.Duration
	AuditFile              string
}

func LoadTradingConfig() (TradingConfig, error) {
	config := TradingConfig{
		Model:                  viper.GetString("trading.model"),
		MaxConcurrentPositions: viper.GetInt("trading.max_concurrent_positions"),
		OpportunitySlotCount:   viper.GetInt("trading.entry.opportunity_slot_count"),
		MaxQuoteAge:            viper.GetDuration("trading.max_quote_age"),
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

	if config.MaxConcurrentPositions <= 0 {
		return fmt.Errorf("config: trading.max_concurrent_positions must be positive")
	}

	if config.OpportunitySlotCount < 0 {
		return fmt.Errorf("config: trading.entry.opportunity_slot_count must not be negative")
	}

	if config.MaxQuoteAge <= 0 {
		return fmt.Errorf("config: trading.max_quote_age must be positive")
	}

	if config.OrderAckTimeout <= 0 {
		return fmt.Errorf("config: trading.order_ack_timeout must be positive")
	}

	if config.EntryTransitTTL <= 0 {
		return fmt.Errorf("config: trading.entry.transit_ttl must be positive")
	}

	if err := ensureParentDirCreatable("trading.audit.file", config.AuditFile); err != nil {
		return err
	}

	return nil
}
