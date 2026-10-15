package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
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
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/data"
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
		system.Epoch.Store(epoch)

		public := network.NewWebsocketClient(ctx)

		if err := public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "collect: public websocket open failed", err,
			))
		}

		var privateTransport broker.Transport

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

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(nmruntime.READY)
		storeTee.Run()
		storeTee.Push(data.NewMeasurement(
			epoch, "epoch", "run", processStartedAt, processStartedAt,
		))

		instrument := broker.NewInstrument(ctx, public, storeTee)
		normalizer := spot.NewNormalizer()

		if err := broker.SeedNormalizer(normalizer); err != nil {
			return err
		}

		book := broker.NewBook(ctx, normalizer)
		book.SetResync(func(symbol string) {
			if resyncErr := instrument.Resync(symbol); resyncErr != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"[collect] book resync failed for "+symbol,
					resyncErr,
				))
			}
		})
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

		correlationSignal := correlation.NewSignal(ctx)
		cvdSignal := cvd.NewSignal(ctx)
		depthflowSignal := depthflow.NewSignal(ctx)
		hawkesSignal := hawkes.NewSignal(ctx)
		leadlagSignal := leadlag.NewSignal(ctx)
		liquiditySignal := liquidity.NewSignal(ctx)
		morphologySignal := morphology.NewSignal(ctx)
		pumpdumpSignal := pumpdump.NewSignal(ctx)
		sentimentSignal := sentiment.NewSignal(ctx)
		toxicitySignal := toxicity.NewSignal(ctx)

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

		for _, runsys := range []nmruntime.RuntimeSystem{
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

		ingressHalted := make(chan error, 1)

		startIngress := func(
			client broker.Transport, name string,
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

					// Checked before channel dispatch: a level3 rejection
					// carries "channel":"level3" (see root.go ingress).
					if rejection := subscribeRejection(buf); rejection != nil {
						softer, isSoft := client.(interface{ SoftDown(error) error })

						if isSoft && name == "level3" {
							errnie.Warn(fmt.Sprintf(
								"[collect] %s subscription rejected, backing off and redialing: %s",
								name, rejection,
							))

							time.Sleep(5 * time.Second)
							err := softer.SoftDown(rejection)
							errnie.Error(err)
							continue
						}

						halt(errnie.Err(
							errnie.NotAcceptable,
							fmt.Sprintf("collect: %s subscription rejected", name),
							rejection,
						))

						return
					}

					channelNode, err := sonic.Get(buf, "channel")

					if err != nil {
						continue
					}

					channel, err := channelNode.StrictString()

					if err != nil {
						continue
					}

					switch channel {
					case "level3":
						if err := handleLevel3(buf, epoch, book, storeTee, name); err != nil {
							if strings.Contains(err.Error(), "awaiting snapshot") || strings.Contains(err.Error(), "checksum mismatch") {
								errnie.Warn(fmt.Sprintf("[collect] %s level3 book update diverged: %s", name, err))
								continue
							}

							halt(err)
							return
						}
					case "trade":
						if err := handleTrade(buf, epoch, price, book, workspace, storeTee, name, nil); err != nil {
							halt(err)
							return
						}
					}
				}
			}()
		}

		startIngress(public, "public")

		// Every socket is read while it subscribes: Kraken rejects Level3
		// snapshot requests on a socket whose earlier snapshots sit unread.
		instrument.SetLevel3Stale(book.Stale)
		instrument.SetLevel3Reader(func(client *network.WebsocketClient) {
			startIngress(client, "level3")
		})

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

				timeout := time.After(15 * time.Second)
				for storeTee.Pending() > 0 {
					select {
					case <-timeout:
						errnie.Warn("[collect] storeTee flush timed out on shutdown")
						return nil
					case <-time.After(100 * time.Millisecond):
					}
				}

				errnie.Info("[collect] final storeTee flush complete")
				return nil
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
