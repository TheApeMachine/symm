package tune

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/replay"
)

func attachInstrumentRules(
	ctx context.Context,
	costs *replay.ReplayCosts,
	rules *broker.InstrumentRulesCache,
) error {
	if costs == nil {
		return fmt.Errorf("optimizer/tune: replay costs are required")
	}

	if rules != nil {
		costs.InstrumentRules = rules

		return nil
	}

	if costs.InstrumentRules != nil {
		return nil
	}

	if !instrumentRulesLoadEnabled() {
		return nil
	}

	loaded, pairCount, err := broker.LoadInstrumentRulesFromKraken(ctx)

	if err != nil {
		return fmt.Errorf("optimizer/tune: %w", err)
	}

	costs.InstrumentRules = loaded
	log.TuneLog("loaded %d Kraken instrument rules for replay", pairCount)

	return nil
}

func instrumentRulesLoadEnabled() bool {
	config := viper.GetViper()

	if !config.IsSet("optimizer.tune.load_instrument_rules") {
		return true
	}

	return config.GetBool("optimizer.tune.load_instrument_rules")
}
