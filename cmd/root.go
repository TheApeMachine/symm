package cmd

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
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

			channel := make(chan []byte, viper.GetInt("system.websocket.channel.buffer"))
			booter := system.NewBooter(ctx, channel)
			persistDir := strings.TrimSpace(viper.GetString("cognitive.persist_dir"))

			if persistDir == "" {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"cognitive.persist_dir is required for position recovery",
					nil,
				))
			}

			if strings.HasPrefix(persistDir, "~/") {
				home, err := os.UserHomeDir()

				if err != nil {
					return errnie.Error(errnie.Err(
						errnie.IO, "failed to resolve cognitive.persist_dir", err,
					))
				}

				persistDir = filepath.Join(home, strings.TrimPrefix(persistDir, "~/"))
			}

			if !filepath.IsAbs(persistDir) {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"cognitive.persist_dir must be absolute or home-relative",
					nil,
				))
			}

			tree := dmt.NewTree(persistDir)

			defer func() {
				errnie.Error(tree.Close())
			}()

			thesis := types.NewThesis(channel)

			if encoded, found := tree.Get([]byte(types.ThesisKey)); found {
				if err := sonic.Unmarshal(encoded, thesis); err != nil {
					return errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"failed to restore persisted Thesis",
						err,
					))
				}
			}

			simulator := websocket.NewLatencySimulator(booter)

			public := websocket.New(
				ctx,
				simulator,
				false,
				websocket.PublicWebSocketURL,
			)

			private := websocket.New(
				ctx,
				simulator,
				true,
				websocket.PrivateWebSocketURL,
			)

			var paper *websocket.Paper

			if viper.GetString("trading.model") == "paper" {
				paper = websocket.NewPaper(ctx, simulator)
			}

			api := websocket.NewAPI(ctx, public, private, paper)
			defer api.Close()

			price := broker.NewPrice(api)
			instrument := broker.NewInstrument(api, price, channel)
			balance := broker.NewBalance(api, channel)
			desk := broker.NewDesk(api, instrument, price, balance, thesis, channel)
			uiHub, err := ui.NewHub(ctx, price, balance, desk, channel)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create UI hub",
					err,
				))
			}

			defer uiHub.Close()

			hawkesSignal := hawkes.NewSignal(ctx, api)
			analyzer, err := logic.NewAnalyzer(ctx, booter, api, hawkesSignal, tree)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create analyzer",
					err,
				))
			}

			planner := strategy.NewPlanner(
				ctx,
				channel,
				[]types.Signal{
					pumpdump.NewSignal(ctx, api),
					liquidity.NewSignal(ctx, api),
					toxicity.NewSignal(ctx, api),
					leadlag.NewSignal(ctx, api),
					cvd.NewSignal(ctx, api),
					correlation.NewSignal(ctx, api),
					exhaust.NewSignal(ctx, api, instrument),
					sentiment.NewSignal(ctx, api),
					depthflow.NewSignal(ctx, api, instrument),
					fluid.NewSignal(ctx, api, instrument),
					hawkesSignal,
				},
				analyzer,
			)

			crypto, err := trader.NewCrypto(
				ctx,
				booter,
				api,
				price,
				balance,
				desk,
				instrument,
				analyzer,
				planner,
				tree,
				thesis,
			)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to create crypto",
					err,
				))
			}

			defer crypto.Close()

			booter.AddStages(
				system.NewStage(
					system.StagePreflight,
					simulator,
					public,
					private,
					paper,
					instrument,
					balance,
					price,
					desk,
				),
				system.NewStage(
					system.StageWarmup,
					crypto,
				),
				system.NewStage(
					system.StageReady,
					analyzer,
					planner,
				),
			)

			if err := booter.Start(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"failed to boot",
					err,
				))
			}

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
