package cmd

import (
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
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
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
			errnie.Apply(&errnie.Config{
				Level: viper.GetString("system.log.level"),
			})

			errnie.Info(fmt.Sprintf("symm started with %d CPUs", runtime.NumCPU()))
			startPprof()

			uiChannel := make(chan []byte, 1024)
			manifoldChannel := make(chan []byte, 1024)

			api := utils.NewWaiter[*websocket.API](websocket.NewAPI(
				cmd.Context(),
				websocket.New(cmd.Context(), nil, false, websocket.PublicWebSocketURL),
				websocket.New(cmd.Context(), nil, false, websocket.PrivateWebSocketURL),
			)).Wait()

			price := utils.NewWaiter[*broker.Price](broker.NewPrice(api)).Wait()

			instrument := utils.NewWaiter[*broker.Instrument](
				broker.NewInstrument(api, price, uiChannel),
			).Wait()

			balance := utils.NewWaiter[*broker.Balance](
				broker.NewBalance(api, uiChannel),
			).Wait()

			desk := utils.NewWaiter[*broker.Desk](broker.NewDesk(
				cmd.Context(),
				api,
				instrument,
				price,
				balance,
				uiChannel,
			)).Wait()

			tree, err := dmt.NewTree("")

			if err != nil {
				return errnie.Error(fmt.Errorf("failed to create decision tree: %w", err))
			}

			analyzer := utils.NewWaiter[*logic.Analyzer](logic.NewAnalyzer(
				cmd.Context(),
				api,
				tree,
				uiChannel,
				manifoldChannel,
				nil,
			)).Wait()

			planner := utils.NewWaiter[*strategy.Planner](strategy.NewPlanner(
				cmd.Context(),
				uiChannel,
				api,
				desk,
				instrument,
				price,
				balance,
				analyzer,
				nil,
			)).Wait()

			utils.NewWaiter[*trader.Crypto](trader.NewCrypto(
				cmd.Context(),
				uiChannel,
				nil,
				planner,
				desk,
			)).Wait()

			utils.NewWaiter[*correlation.Signal](correlation.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*cvd.Signal](cvd.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*depthflow.Signal](depthflow.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*exhaust.Signal](exhaust.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*hawkes.Signal](hawkes.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*leadlag.Signal](leadlag.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*liquidity.Signal](liquidity.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*pumpdump.Signal](pumpdump.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*sentiment.Signal](sentiment.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()
			utils.NewWaiter[*toxicity.Signal](toxicity.NewSignal(cmd.Context(), api, planner, uiChannel)).Wait()

			hub := ui.NewHub(
				cmd.Context(),
				desk,
				price,
				balance,
				uiChannel,
				manifoldChannel,
			)

			return hub.Serve()
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

	// Live watching is disabled until an atomic config generation swap exists.
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
