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
	PrimarySlotCount       int
	PrimarySlotFraction    float64
	SecondarySlotFraction  float64
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
		PrimarySlotCount:       viper.GetInt("trading.entry.primary_slot_count"),
		PrimarySlotFraction:    viper.GetFloat64("trading.position_fraction"),
		SecondarySlotFraction:  viper.GetFloat64("trading.entry.secondary_slot_fraction"),
		MaxConcurrentPositions: viper.GetInt("trading.max_concurrent_positions"),
		MaxQuoteAge:            viper.GetDuration("trading.max_quote_age"),
		MaxSpreadBps:           viper.GetFloat64("trading.max_spread_bps"),
		MaxSlippageBps:         viper.GetFloat64("trading.max_slippage_bps"),
		OrderAckTimeout:        viper.GetDuration("trading.order_ack_timeout"),
		EntryTransitTTL:        viper.GetDuration("trading.entry.transit_ttl"),
		AuditFile:              viper.GetString("trading.audit.file"),
	}

	if config.PrimarySlotCount <= 0 {
		config.PrimarySlotCount = 2
	}

	if config.PrimarySlotFraction <= 0 {
		config.PrimarySlotFraction = config.PositionFraction
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

	if err := requireUnitInterval("trading.position_fraction", config.PositionFraction); err != nil {
		return err
	}

	if config.SecondarySlotFraction <= 0 {
		return fmt.Errorf("config: trading.entry.secondary_slot_fraction must be positive")
	}

	if err := requireUnitInterval(
		"trading.entry.secondary_slot_fraction",
		config.SecondarySlotFraction,
	); err != nil {
		return err
	}

	if config.SecondarySlotFraction > config.PositionFraction {
		return fmt.Errorf(
			"config: trading.entry.secondary_slot_fraction must not exceed position_fraction",
		)
	}

	if err := requireUnitInterval(
		"trading.entry.primary_slot_fraction",
		config.PrimarySlotFraction,
	); err != nil {
		return err
	}

	if config.PrimarySlotFraction > config.PositionFraction {
		return fmt.Errorf(
			"config: trading.entry.primary_slot_fraction must not exceed position_fraction",
		)
	}

	if config.PrimarySlotCount <= 0 {
		return fmt.Errorf("config: trading.entry.primary_slot_count must be positive")
	}

	if config.PrimarySlotCount > config.MaxConcurrentPositions {
		return fmt.Errorf(
			"config: trading.entry.primary_slot_count must not exceed max_concurrent_positions",
		)
	}

	if config.MaxConcurrentPositions <= 0 {
		return fmt.Errorf("config: trading.max_concurrent_positions must be positive")
	}

	if config.MaxQuoteAge <= 0 {
		return fmt.Errorf("config: trading.max_quote_age must be positive")
	}

	if err := requirePositiveBasisPoints(
		"trading.max_spread_bps",
		config.MaxSpreadBps,
	); err != nil {
		return err
	}

	if err := requirePositiveBasisPoints(
		"trading.max_slippage_bps",
		config.MaxSlippageBps,
	); err != nil {
		return err
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
