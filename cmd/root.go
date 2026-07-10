package cmd

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
)

/*
Embed a mini filesystem into the binary to hold the default config file.
This will be written to the home directory of the user running the service,
which allows a developer to easily override the config file.
*/
//go:embed cfg/config.yml
var embedded embed.FS

var (
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))
			startPprof()

			tree := dmt.NewTree(viper.GetString("cognitive.persist_dir"))

			pool := qpool.NewQ[any](ctx, runtime.NumCPU(), runtime.NumCPU(), nil)

			public := websocket.New(
				ctx, 
				pool, 
				"ws.kraken.com/v2", 
				"https://api.kraken.com/v2", 
				false, 
				true,
			)

			defer public.Close()

			private := websocket.New(
				ctx, 
				pool, 
				"ws-auth.kraken.com/v2", 
				"https://api.kraken.com/v2", 
				true, 
				false,
			)
			
			defer private.Close()

			level3 := websocket.New(
				ctx, 
				pool, 
				"ws-l3.kraken.com/v2", 
				"https://api.kraken.com/v2", 
				true, 
				false,
			)
			
			defer level3.Close()

			uiHub, err := ui.NewHub(ctx)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create UI hub",
					err,
				))
			}

			defer uiHub.Close()

			crypto, err := trader.NewCrypto(
				ctx,
				pool,
				tree,
				private,
				public,
				uiHub,
				level3,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create crypto",
					err,
				))
			}

			defer crypto.Close()
			crypto.Run()

			return uiHub.Serve()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func startPprof() {
	if !viper.GetBool("system.pprof.enabled") && os.Getenv("SYMM_PPROF") == "" {
		return
	}

	addr := viper.GetString("system.pprof.addr")

	if addr == "" {
		addr = "127.0.0.1:6060"
	}

	go func() {
		errnie.Error(http.ListenAndServe(addr, nil))
	}()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: try cmd/cfg/config.yml, ./config.yml, $HOME/.symm/config.yml, then embedded default)",
	)
}

func initConfig() {
	viper.SetConfigType("yml")

	tryRead := func(path string) error {
		viper.SetConfigFile(path)
		return viper.ReadInConfig()
	}

	loaded := false

	if rootCmd.PersistentFlags().Changed("config") && strings.TrimSpace(cfgFile) != "" {
		if err := tryRead(cfgFile); err == nil {
			loaded = true
		} else {
			fmt.Fprintf(os.Stderr, "symm: config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}
	}

	if !loaded {
		paths := []string{
			"cmd/cfg/config.yml",
			"config.yml",
		}

		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".symm", "config.yml"))
		}

		for _, p := range paths {
			if err := tryRead(p); err == nil {
				loaded = true
				break
			}
		}
	}

	if !loaded {
		cfgReader, err := embedded.Open("cfg/config.yml")

		if err != nil {
			fmt.Fprintf(os.Stderr, "embedded config file not readable: %v\n", err)
			os.Exit(1)
		}

		defer cfgReader.Close()

		if readErr := viper.ReadConfig(cfgReader); readErr != nil {
			fmt.Fprintf(os.Stderr, "embedded config file not readable: %v\n", readErr)
			os.Exit(1)
		}
	}

	viper.WatchConfig()
}

const rootLong = `
Shake your money maker like somebody's 'bout to pay ya
Don't worry about them haters, keep your nose up in the ayer
You know I got it, if you wanna come get it
Stand next to this money like - ey ey ey

Shake, shake, shake your money maker
Like you were shaking it for some paper

...
`
