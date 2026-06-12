package cmd

import (
	"os"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/config"
)

func liveReadinessDependencies(
	tradingConfig config.TradingConfig,
) config.LiveReadinessDependencies {
	return config.LiveReadinessDependencies{
		APIKey:    os.Getenv("SYMM_KRAKEN_API_KEY"),
		APISecret: os.Getenv("SYMM_KRAKEN_API_SECRET"),
		AuditErr:  liveAuditErr(tradingConfig.AuditFile),
	}
}

func liveAuditErr(path string) error {
	recorder, err := audit.NewRecorder(path)

	if err != nil {
		return err
	}

	return recorder.Close()
}
