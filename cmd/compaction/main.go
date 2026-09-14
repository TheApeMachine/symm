package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
)

func main() {
	viper.SetDefault("storage.s3.endpoint", "http://seaweed.home.arpa:8333")
	viper.SetDefault("storage.iceberg.uri", "http://iceberg.seaweed.home.arpa")
	viper.SetDefault("storage.iceberg.warehouse", "s3://symmtables/")

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath("cmd/cfg")
	viper.AddConfigPath("$HOME/.symm")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		errnie.Info(fmt.Sprintf("compaction: config file notice: %v", err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	catalog := tables.Open(ctx)

	if catalog == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"compaction: failed to connect to Iceberg catalog",
			nil,
		))
		os.Exit(1)
	}

	errnie.Info("compaction: starting Iceberg table compaction across all families...")

	if err := catalog.CompactAll(ctx); err != nil {
		errnie.Error(err)
		os.Exit(1)
	}

	errnie.Info("compaction: completed successfully.")
}
