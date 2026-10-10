package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/morphology"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/system"
)

// collectCmd represents the collect command for continuous, decoupled market ingress
var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect raw market data and sensory measurements continuously",
	Long:  collectLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		errnie.Apply(&errnie.Config{
			Level: viper.GetString("system.log.level"),
		})

		// This process signs Kraken REST calls with its own role's key pair.
		if err := kraken.UseCredentials(kraken.RoleCollector); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "collect: kraken credentials unavailable", err,
			))
		}

		_, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "symm.collect.theapemachine.app",
			ServerAddress:   "http://localhost:4040",
			Logger:          nil,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[collect] error starting pyroscope profiler",
				err,
			))
		}

		errnie.Info(fmt.Sprintf(
			"[collect] symm collector started with %d CPUs", runtime.NumCPU(),
		))

		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		epoch := processStartedAt.UnixNano()

		catalog := tables.Open(ctx)

		if catalog == nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "collect: catalog initialization failed", nil,
			))
		}

		if err := catalog.Ensure(ctx); err != nil {
			return err
		}

		if err := catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: processStartedAt.UTC(),
			Status:    "collecting",
		}); err != nil {
			errnie.Warn("[collect] failed to record hindsight run: " + err.Error())
		}

		public := network.NewWebsocketClient(ctx)

		if err := public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "collect: public websocket open failed", err,
			))
		}

		var privateTransport interface {
			broker.Transport
			Read() ([]byte, error)
		}

		if system.Cfg.Market.Model == "paper" {
			privateTransport = broker.NewPaper(ctx)
		}

		if privateTransport == nil {
			privateWS := network.NewWebsocketClient(ctx)

			if err := privateWS.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "collect: private websocket open failed", err,
				))
			}

			privateTransport = privateWS
		}

		instrument := broker.NewInstrument(public)
		normalizer := spot.NewNormalizer()

		if err := broker.SeedNormalizer(normalizer); err != nil {
			return err
		}

		book := broker.NewBook(ctx, normalizer)
		price := broker.NewPrice(ctx, book, privateTransport, instrument, normalizer)

		if err := instrument.Error(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"collect: instrument registry failed during construction",
				err,
			))
		}

		if err := price.GetFees(instrument.Symbols()); err != nil {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"[collect] initial fees are not available",
				nil,
			))
		}

		if price.Status() != nmruntime.READY {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"[collect] initial price fees are not ready",
				nil,
			))
		}

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")

		correlationSignal := correlation.NewSignal(ctx)
		cvdSignal := cvd.NewSignal(ctx)
		depthflowSignal := depthflow.NewSignal(ctx, book)
		hawkesSignal := hawkes.NewSignal(ctx)
		leadlagSignal := leadlag.NewSignal(ctx)
		liquiditySignal := liquidity.NewSignal(ctx, book)
		morphologySignal := morphology.NewSignal(ctx, book)
		pumpdumpSignal := pumpdump.NewSignal(ctx, book)
		sentimentSignal := sentiment.NewSignal(ctx)
		toxicitySignal := toxicity.NewSignal(ctx, book)

		pipelineNodes := []nmruntime.RuntimeSystem{
			correlationSignal,
			cvdSignal,
			depthflowSignal,
			hawkesSignal,
			leadlagSignal,
			liquiditySignal,
			morphologySignal,
			pumpdumpSignal,
			sentimentSignal,
			toxicitySignal,
		}

		for _, node := range pipelineNodes {
			if node.Status() == nmruntime.ERROR || node.Status() == nmruntime.FATAL {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("collect: %s failed construction", node.Name()),
					node.Error(),
				))
			}
		}

		workspace := nmruntime.NewWorkspace(
			ctx,
			2,
			"workspace",
			[][]nmruntime.Node{
				{
					correlationSignal,
					cvdSignal,
					depthflowSignal,
					hawkesSignal,
					leadlagSignal,
					liquiditySignal,
					morphologySignal,
					pumpdumpSignal,
					sentimentSignal,
					toxicitySignal,
				},
			},
			storeTee,
		)

		instrument.SetLevel3Stale(book.Stale)

		if err := instrument.Subscribe(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"collect: subscribe to instrument universe",
				err,
			))
		}

		if instrument.Status() != nmruntime.READY {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"collect: instrument universe is not seeded",
				nil,
			))
		}

		for _, runsys := range []nmruntime.RuntimeSystem{
			storeTee,
			correlationSignal,
			cvdSignal,
			depthflowSignal,
			hawkesSignal,
			leadlagSignal,
			liquiditySignal,
			morphologySignal,
			pumpdumpSignal,
			sentimentSignal,
			toxicitySignal,
			workspace,
		} {
			runsys.Transition(nmruntime.READY)
		}

		drainErrors := make(chan error, 1)

		go func() {
			drainErrors <- catalog.Drain(ctx, epoch, storeTee)
		}()

		ingressHalted := make(chan error, 1)

		startIngress := func(
			client interface{ Read() ([]byte, error) }, name string,
		) {
			go func() {
				errnie.Info(fmt.Sprintf("[collect] starting %s ingress", name))

				halt := func(err error) {
					select {
					case ingressHalted <- err:
					default:
					}
				}

				for {
					select {
					case <-ctx.Done():
						return
					default:
					}

					buf, err := client.Read()

					if err != nil {
						if closed, ok := client.(interface {
							Context() context.Context
							Error(...error) error
						}); ok && closed.Context().Err() != nil {
							if ctx.Err() != nil {
								return
							}

							halt(errnie.Err(
								errnie.IO,
								fmt.Sprintf("collect: %s ingress transport closed", name),
								errors.Join(closed.Error(), err),
							))

							return
						}

						continue
					}

					channelNode, err := sonic.Get(buf, "channel")

					if err != nil {
						if rejection := subscribeRejection(buf); rejection != nil {
							halt(errnie.Err(
								errnie.NotAcceptable,
								fmt.Sprintf("collect: %s subscription rejected", name),
								rejection,
							))

							return
						}

						continue
					}

					channel, err := channelNode.StrictString()

					if err != nil {
						continue
					}

					switch channel {
					case "level3":
						if err := handleLevel3(buf, epoch, book, storeTee, name); err != nil {
							halt(err)
							return
						}
					case "trade":
						if err := handleTrade(buf, epoch, price, workspace, storeTee, name, nil); err != nil {
							halt(err)
							return
						}
					}
				}
			}()
		}

		startIngress(public, "public")

		instrument.Level3.Range(func(key, value any) bool {
			if client, ok := value.(*network.WebsocketClient); ok {
				startIngress(client, "level3")
			}

			return true
		})

		haltWatched := append(
			[]nmruntime.RuntimeSystem{instrument}, pipelineNodes...,
		)

		nodeHalted := make(chan nmruntime.RuntimeSystem, len(haltWatched))

		for _, node := range haltWatched {
			go func() {
				select {
				case <-ctx.Done():
				case <-node.Context().Done():
					nodeHalted <- node
				}
			}()
		}

		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				errnie.Info("[collect] shutdown requested, flushing in-flight measurements...")

				select {
				case err := <-drainErrors:
					if err != nil {
						return errnie.Error(errnie.Err(
							errnie.IO, "collect: final catalog commit failed", err,
						))
					}

					errnie.Info("[collect] final catalog flush complete")
				case <-time.After(15 * time.Second):
					errnie.Warn("[collect] catalog flush timed out on shutdown")
				}

				return nil
			case err := <-drainErrors:
				return errnie.Error(errnie.Err(
					errnie.IO, "collect: catalog drain stopped", err,
				))
			case err := <-ingressHalted:
				if ctx.Err() != nil {
					return nil
				}

				return errnie.Error(err)
			case node := <-nodeHalted:
				if ctx.Err() != nil {
					return nil
				}

				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("collect: %s halted", node.Name()),
					node.Error(),
				))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(collectCmd)
}

var collectLong = `
Collect raw market data continuously and write sensory measurements directly into Iceberg.
This headless daemon gathers unbroken trades, Level 3 book events, and 12-channel sensory
workspace measurements for the shared historical archive without UI or trading dependencies.
`
