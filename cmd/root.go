package cmd

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	kraken "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/runstats"
	"github.com/theapemachine/symm/runtime"
	"github.com/theapemachine/symm/signal/causal"
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
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/ui"
)

/*
Embed a mini filesystem into the binary to hold the default config file.
This will be written to the home directory of the user running the service,
which allows a developer to easily override the config file.
*/
//go:embed cfg/config.yml cfg/infra.yml cfg/strategy.yml
var embedded embed.FS

// defaultCapturePath is where `make run --record` writes measurements and where
// `make tune` reads them back — the single capture file the two commands share.
const defaultCapturePath = "runs/capture.jsonl"

var (
	cfgFile   string
	recordRun bool

	rootCmd = &cobra.Command{
		Use:   "symm",
		Short: "S.Y.M.M. is not financial advice.",
		Long:  rootLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			errnie.Apply(&errnie.Config{
				Level: viper.GetViper().GetString("system.log.level"),
			})

			// --record guarantees the run collects data for the optimizer, without
			// depending on trading.record.file being set in the config.
			if recordRun {
				viper.Set("trading.record.file", defaultCapturePath)
				errnie.Info("recording run measurements to "+defaultCapturePath+" (feeds `make tune`)", "engine")
			}

			pool := qpool.NewQ[any](cmd.Context(), 1, 4, nil)
			engine, err := NewEngine(cmd.Context(), pool)

			if err != nil {
				return err
			}

			systemCtx := engine.Context()
			services, err := runtime.New(systemCtx, pool)

			if err != nil {
				return err
			}

			streams := focus.NewSet()
			auditWriter, err := services.OpenAudit()

			if err != nil {
				return err
			}

			// Stamp the EFFECTIVE config (post-merge, post-flags) into the run
			// directory and the audit prologue: yesterday's audit shows gates
			// (spread 60bps) that no checked-in config contains — runs and tunes
			// must be attributable to the exact configuration they executed.
			stampEffectiveConfig(auditWriter)

			crypto := trader.NewCryptoWithCaches(
				systemCtx,
				pool,
				streams,
				services.Quotes,
				services.Stress,
				services.Rules,
				auditWriter,
			)

			if crypto == nil {
				return fmt.Errorf("engine: trader construction failed")
			}

			story, err := newStoryWithBookCapture(systemCtx, pool, services, crypto, auditWriter)

			if err != nil {
				return err
			}

			errnie.Info(
				"engine registering systems trading.model="+viper.GetString("trading.model"),
				"engine",
			)

			// Silence must be loud: the watchdog pages (and halts order flow) when
			// inputs that should be flowing aren't — the failure shape of the
			// 2026-06-07 quote-cache severance, which ran blind for an hour.
			watchdog := runstats.NewWatchdog(systemCtx, 10*time.Second, func(name, detail string) {
				errnie.Error(fmt.Errorf("watchdog trip: %s — halting order flow: %s", name, detail))
				crypto.Desk().TripHalt()
			})

			watchdog.Expect("quote-cache-ingest", 30*time.Second, true, runstats.RateExpectation(
				func() uint64 {
					_, ingested := services.Quotes.IngestStats()

					return ingested
				},
				public.RawFramesPublished,
			))

			watchdog.Expect("audit-writer", 0, true, func() (bool, string) {
				if err := auditWriter.Err(); err != nil {
					return false, err.Error()
				}

				return true, ""
			})

			// The desk fail-closes per order when a pair has no instrument rules
			// ("missing instrument rules" on every entry, observed live). Seed the
			// full REST catalog at startup — the same source tune loads — and let
			// the websocket snapshot keep it fresh; the watchdog screams if the
			// cache is still empty once the engine should be warm.
			go seedInstrumentRules(systemCtx, services.Rules)

			watchdog.Expect("instrument-rules", 90*time.Second, false, func() (bool, string) {
				if size := services.Rules.Size(); size == 0 {
					return false, "no instrument rules — every entry will be rejected"
				}

				return true, ""
			})

			// One expectation per signal: a source that publishes nothing for
			// five minutes while raw frames flow is dead, whatever killed it —
			// input starvation, a swallowed warmup error, a parked goroutine.
			// Five signals died silently over two days before this existed.
			for _, source := range types.AllSignalSources() {
				source := source

				watchdog.Expect(
					"signal-"+source.String(),
					5*time.Minute,
					false,
					runstats.RateExpectation(
						func() uint64 { return types.SourceEmissions(source) },
						public.RawFramesPublished,
					),
				)
			}

			apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
			apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

			if err := engine.AddSystems(
				ui.NewHub(systemCtx, pool),
				public.NewWebSocket(systemCtx, pool, streams),
				watchdog,
			); err != nil {
				return err
			}

			for _, executionSystem := range private.ExecutionSystems(
				systemCtx, pool, apiKey, apiSecret, services.Quotes,
			) {
				if err := engine.AddSystems(executionSystem); err != nil {
					return err
				}
			}

			if err := engine.AddSystems(
				kraken.NewInstrument(systemCtx, pool),
				causal.NewSignal(systemCtx, pool),
				correlation.NewSignal(systemCtx, pool),
				cvd.NewSignal(systemCtx, pool),
				depthflow.NewSignal(systemCtx, pool),
				exhaust.NewSignal(systemCtx, pool),
				fluid.NewSignal(systemCtx, pool),
				hawkes.NewSignal(systemCtx, pool),
				leadlag.NewSignal(systemCtx, pool),
				liquidity.NewSignal(systemCtx, pool),
				pumpdump.NewSignal(systemCtx, pool),
				sentiment.NewSignal(systemCtx, pool),
				toxicity.NewToxicity(systemCtx, pool),
				story,
				crypto,
			); err != nil {
				return err
			}

			errnie.Info("engine.Start", "engine")
			return engine.Start()
		},
	}
)

func Execute() {
	err := rootCmd.Execute()

	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"config",
		"",
		"path to config file (default: try cmd/cfg/config.yml, ./config.yml, $HOME/.symm/config.yml, then embedded default)",
	)

	rootCmd.PersistentFlags().BoolVar(
		&recordRun,
		"record",
		false,
		"record run measurements to "+defaultCapturePath+" so `make tune` can optimize on them",
	)
}

func initConfig() {
	if rootCmd.PersistentFlags().Changed("config") && strings.TrimSpace(cfgFile) != "" {
		if err := mergeConfigFiles(cfgFile); err != nil {
			fmt.Fprintf(os.Stderr, "symm: config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}

		return
	}

	if err := loadDefaultConfigs(); err != nil {
		fmt.Fprintf(os.Stderr, "symm: config: %v\n", err)
		os.Exit(1)
	}
}

/*
seedInstrumentRules fills the live rules cache from REST AssetPairs with a few
retries, so the desk can validate orders for every tradable pair from the first
minute instead of waiting on (or missing) the websocket instrument snapshot.
*/
func seedInstrumentRules(ctx context.Context, rules *broker.InstrumentRulesCache) {
	for attempt := 1; attempt <= 3; attempt++ {
		loaded, err := rules.SeedFromKraken(ctx)

		if err == nil {
			errnie.Info(fmt.Sprintf("seeded %d instrument rules from REST", loaded), "engine")

			return
		}

		errnie.Error(fmt.Errorf("instrument rules seed attempt %d/3: %w", attempt, err))

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}

	errnie.Error(fmt.Errorf("instrument rules REST seed failed — relying on the websocket snapshot alone"))
}

/*
stampEffectiveConfig persists the merged viper state with a content hash so every
capture, audit trail, and tune is attributable to the exact configuration that
produced it.
*/
func stampEffectiveConfig(auditWriter *audit.Writer) {
	raw, err := json.MarshalIndent(viper.AllSettings(), "", "  ")

	if err != nil {
		errnie.Error(err)

		return
	}

	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:8])

	if err := os.MkdirAll("runs", 0o755); err != nil {
		errnie.Error(err)

		return
	}

	path := "runs/effective-config.json"

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		errnie.Error(err)

		return
	}

	errnie.Info("effective config "+hash+" -> "+path, "engine")

	if auditWriter != nil {
		if err := auditWriter.Write(map[string]any{
			"audit_event": "session_config",
			"config_sha":  hash,
			"path":        path,
		}); err != nil {
			errnie.Error(err)
		}
	}
}

func newStoryWithBookCapture(
	ctx context.Context,
	pool *qpool.Q[any],
	services *runtime.Runtime,
	crypto *trader.Crypto,
	auditWriter *audit.Writer,
) (*market.Story, error) {
	bookEnricher, err := broker.MeasurementBookEnricher(services.Quotes)

	if err != nil {
		return nil, fmt.Errorf("engine: measurement book enricher: %w", err)
	}

	story := market.NewStoryWithAudit(ctx, pool, auditWriter)

	if story == nil {
		return nil, fmt.Errorf("engine: story construction failed")
	}

	story.SetBookEnricher(bookEnricher)
	story.SetQuoteReady(func(symbol string) bool {
		_, ok := services.Quotes.Snapshot(symbol)

		return ok
	})
	story.SetPositionHeld(crypto.SymbolHeld)

	return story, nil
}

const rootLong = `
Shake your money maker like somebody's 'bout to pay ya
I see you on my radar, don't you act like you're afraid of shit
You know I got it, If you wanna come get it
Stand next to this money like - ey ey ey
Shake your money maker like somebody's 'bout to pay ya
Don't worry about them haters, keep your nose up in the air
You know I got it, If you wanna come get it
Stand next to this money like - ey ey ey

Shake, shake, shake your money maker
Like you were shaking it for some paper

...
`
