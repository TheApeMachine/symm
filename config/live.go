package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const LiveConfirmPhrase = "I_UNDERSTAND_THIS_CAN_PLACE_REAL_ORDERS"

type LiveReadinessConfig struct {
	Confirm                       string
	APIKeyPermissionsConfirmed    bool
	ClockSynchronized             bool
	ExchangeConnectivityConfirmed bool
	PaperLiveParityPassed         bool
	MaxOrderNotional              float64
	MaxDailyLoss                  float64
}

type LiveReadinessDependencies struct {
	APIKey    string
	APISecret string
	AuditErr  error
}

func LoadLiveReadinessConfig() LiveReadinessConfig {
	return LiveReadinessConfig{
		Confirm:                       viper.GetString("live.confirm"),
		APIKeyPermissionsConfirmed:    viper.GetBool("live.api_key_permissions_confirmed"),
		ClockSynchronized:             viper.GetBool("live.clock_synchronized"),
		ExchangeConnectivityConfirmed: viper.GetBool("live.exchange_connectivity_confirmed"),
		PaperLiveParityPassed:         viper.GetBool("live.paper_live_parity_passed"),
		MaxOrderNotional:              viper.GetFloat64("live.max_order_notional"),
		MaxDailyLoss:                  viper.GetFloat64("live.max_daily_loss"),
	}
}

func CheckLiveReadiness(
	tradingConfig TradingConfig,
	liveConfig LiveReadinessConfig,
	dependencies LiveReadinessDependencies,
) error {
	if tradingConfig.Model != "live" {
		return nil
	}

	readinessErrs := []error{
		requireLiveConfirm(liveConfig.Confirm),
		requireLiveCredential("SYMM_KRAKEN_API_KEY", dependencies.APIKey),
		requireLiveCredential("SYMM_KRAKEN_API_SECRET", dependencies.APISecret),
		requireLiveFlag(
			"live.api_key_permissions_confirmed",
			liveConfig.APIKeyPermissionsConfirmed,
		),
		requireLiveRiskLimit("live.max_order_notional", liveConfig.MaxOrderNotional),
		requireLiveRiskLimit("live.max_daily_loss", liveConfig.MaxDailyLoss),
		requireLiveAudit(dependencies.AuditErr),
		requireLiveFlag("live.clock_synchronized", liveConfig.ClockSynchronized),
		requireLiveFlag(
			"live.exchange_connectivity_confirmed",
			liveConfig.ExchangeConnectivityConfirmed,
		),
		requireLiveFlag("live.paper_live_parity_passed", liveConfig.PaperLiveParityPassed),
		requirePositiveInt("trading.max_concurrent_positions", tradingConfig.MaxConcurrentPositions),
	}

	return errors.Join(readinessErrs...)
}

func requireLiveConfirm(confirm string) error {
	if confirm == LiveConfirmPhrase {
		return nil
	}

	return fmt.Errorf(
		"live readiness: live.confirm must equal %q",
		LiveConfirmPhrase,
	)
}

func requireLiveCredential(name string, value string) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}

	return fmt.Errorf("live readiness: %s is required", name)
}

func requireLiveFlag(name string, enabled bool) error {
	if enabled {
		return nil
	}

	return fmt.Errorf("live readiness: %s must be true", name)
}

func requireLiveRiskLimit(name string, value float64) error {
	if value > 0 {
		return nil
	}

	return fmt.Errorf("live readiness: %s must be positive", name)
}

func requireLiveAudit(auditErr error) error {
	if auditErr == nil {
		return nil
	}

	return fmt.Errorf("live readiness: audit writer is not healthy: %w", auditErr)
}

func requirePositiveInt(name string, value int) error {
	if value > 0 {
		return nil
	}

	return fmt.Errorf("live readiness: %s must be positive", name)
}
