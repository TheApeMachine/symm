package cmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/cognitive/dmt"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
)

var optimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Replay captured Kraken websocket frames through candidate playbooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		errnie.Apply(&errnie.Config{
			Level: viper.GetViper().GetString("system.log.level"),
		})

		errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))
		startPprof()

		tree := dmt.NewTree(viper.GetString("cognitive.persist_dir"))

		publicSocket := websocket.NewPublic(ctx, nil)
		defer publicSocket.Close()

		accountSource := websocket.NewPrivateAccount(ctx)
		defer accountSource.Close()

		uiHub, err := ui.NewHub(ctx)
		if err != nil {
			cancel()
			return errnie.Error(errnie.Err(
				errnie.IO,
				"ui: failed to create hub",
				err,
			))
		}

		defer uiHub.Close()

		cryptoTrader, err := trader.NewCrypto(ctx, tree, uiHub, accountSource, publicSocket)

		if err != nil {
			cancel()
			return errnie.Error(errnie.Err(
				errnie.IO,
				"trader: failed to create crypto",
				err,
			))
		}

		defer cryptoTrader.Close()

		accountBridge := newAccountBridge(
			ctx,
			accountSource,
			cryptoTrader,
			uiHub,
			viper.GetDuration("ui.heartbeat_interval"),
		)

		if err := accountBridge.Start(); err != nil {
			cancel()
			return errnie.Error(err)
		}

		defer accountBridge.Close()

		go func() {
			if err := cryptoTrader.Run(); err != nil {
				errnie.Error(err)
				cancel()
			}
		}()

		<-ctx.Done()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(optimizeCmd)
}
