package types

import (
	"github.com/spf13/viper"
)

/*
ShardWorkers returns the configured per-stage symbol concurrency. Tests and
fixtures that never load a config file fall back to one serial worker so their
ordering stays deterministic.
*/
func ShardWorkers() int {
	workers := viper.GetInt("system.streaming.symbol_shards")

	if workers < 1 {
		return 1
	}

	return workers
}
