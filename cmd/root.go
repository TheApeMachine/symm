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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
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

			buffer := viper.GetInt("system.websocket.channel.buffer")
			errnie.Info(fmt.Sprintf("starting ui channel with a buffer of %d", buffer))
			channel := make(chan []byte, buffer)

			booter := system.NewBooter(ctx, channel)
			inMemory := viper.GetBool("cognitive.in_memory")
			persistDir := strings.TrimSpace(viper.GetString("cognitive.persist_dir"))

			if !inMemory {
				if persistDir == "" {
					return errnie.Error(errnie.Err(
						errnie.Validation,
						"cognitive.persist_dir is required unless cognitive.in_memory is set",
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

				if err := rotateCognitive(persistDir); err != nil {
					return errnie.Error(err)
				}
			}

			treeDir := persistDir

			if inMemory {
				treeDir = ""
				errnie.Info("cognitive tree running in-memory (no DMT WAL)")
			}

			tree := dmt.NewTree(treeDir)

			defer func() {
				errnie.Error(tree.Close())
			}()

			simulator := websocket.NewLatencySimulator(booter)

			dataPath := strings.TrimSpace(viper.GetString("system.data_path"))

			if strings.HasPrefix(dataPath, "~/") {
				home, err := os.UserHomeDir()

				if err != nil {
					return errnie.Error(errnie.Err(
						errnie.IO, "failed to resolve system.data_path", err,
					))
				}

				dataPath = filepath.Join(home, strings.TrimPrefix(dataPath, "~/"))
			}

			auditDir := persistDir

			if inMemory || auditDir == "" {
				auditDir = dataPath
			}

			recorder, err := audit.NewRecorder(filepath.Join(
				auditDir, "runtime-audit.jsonl",
			))

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "failed to create runtime audit recorder", err,
				))
			}

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

			if err := os.MkdirAll(dataPath, 0o700); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "failed to create data directory", err,
				))
			}

			thesis := types.NewThesis(channel, nil)
			encoded, err := os.ReadFile(filepath.Join(dataPath, "thesis.json"))

			if err == nil {
				thesis = restoreThesis(
					thesis, channel, encoded, "failed to unmarshal optional thesis",
				)
			} else if !os.IsNotExist(err) {
				errnie.Error(errnie.Err(
					errnie.IO, "failed to read optional thesis from data directory", err,
				))
			}

			// Wallet is inventory authority. thesis.json may still carry stale
			// OPEN lots from prior runs; purge them before Balance/UI seed.
			thesis.Holdings.Range(func(key, value any) bool {
				thesis.Holdings.Delete(key)
				return true
			})

			wired, err := stack.Boot(ctx, api, stack.Options{
				Booter:         booter,
				Paper:          paper,
				Signals:        productionSignals,
				Channel:        channel,
				Tree:           tree,
				Thesis:         thesis,
				Recorder:       recorder,
				PreflightExtra: []types.StatusReporter{simulator, public, private},
				AttachUI: func(
					stackBooter *system.Booter,
					price *broker.Price,
					balance *broker.Balance,
					stackThesis *types.Thesis,
					stackChannel chan []byte,
				) (*ui.Hub, error) {
					warmupReady := func() bool {
						return stackBooter.Ready(system.StageWarmup)
					}
					hub, hubErr := ui.NewHub(
						ctx, price, balance, stackThesis, stackChannel, warmupReady,
					)

					if hubErr != nil {
						return nil, hubErr
					}

					errnie.AttachWriter(ui.NewErrorBridge(hub, warmupReady))

					return hub, nil
				},
			})

			if err != nil {
				return errnie.Error(err)
			}

			defer wired.Close()

			if wired.UIHub == nil {
				return errnie.Error(errnie.Err(
					errnie.Internal, "ui hub missing after boot", nil,
				))
			}

			defer wired.UIHub.Close()

			wired.Crypto.Run()

			return wired.UIHub.Serve()
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
