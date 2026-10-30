package cmd

import (
	"context"
	"fmt"
	"io"
	sysruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/disconnect"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/network"
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
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

type hubDeskExiter struct {
	desk   *broker.Desk
	record func(*broker.Position)
	path   func(string, *broker.Position) string
}

func (exiter hubDeskExiter) Exit(symbol string, virtual bool) error {
	position := exiter.desk.Position(symbol)
	path := ""
	if position != nil {
		if exiter.path != nil {
			path = exiter.path(symbol, position)
		}
		if path == "" {
			path = position.EntryPath
		}
	}

	if err := exiter.desk.Exit(symbol, path, virtual); err != nil {
		return err
	}

	if virtual && exiter.record != nil && position != nil {
		exiter.record(position)
	}

	return nil
}

/*
Booter encapsulates the bootstrapping, component assembly, and supervision
lifecycle for daemon processes (such as symm collect).
*/
type Booter struct {
	*nmruntime.System
	epoch            int64
	public           *network.WebsocketClient
	privateTransport broker.Transport
	instrument       *broker.Instrument
	book             *broker.Book
	price            *broker.Price
	balance          *broker.Balance
	desk             *broker.Desk
	normalizer       *spot.Normalizer
	signals          []nmruntime.Node
	training         *strategy.Training
	workspace        *nmruntime.Workspace
	store            *store.Client
	replies          sync.Map
}

func NewBooter(ctx context.Context, name string) *Booter {
	errnie.Apply(&errnie.Config{
		Level: viper.GetString("system.log.level"),
	})

	// Paper signs with the collector key. Live orders use the main key so
	// they do not share a nonce stream with a collector process.
	role := kraken.RoleCollector
	if viper.GetString("market.model") == "real" {
		role = kraken.RoleMain
	}

	if err := kraken.UseCredentials(role); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[booter] kraken credentials unavailable", err,
		))
	}

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "symm." + name + ".theapemachine.app",
		ServerAddress:   "http://localhost:4040",
		Logger:          nil,
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[booter] error starting pyroscope profiler",
			err,
		))
	}

	return &Booter{
		System: nmruntime.NewSystem(ctx, name, nil),
		store:  store.NewClient(ctx, "symm"),
	}
}

func (booter *Booter) Run() error {
	errnie.Info(fmt.Sprintf(
		"[booter] symm boot started with %d CPUs", sysruntime.NumCPU(),
	))

	started := time.Now()

	// Transports and the market come first: training paper-trades through the
	// price and the desk. The workspace is up before any trade is subscribed.
	booter.Transition(nmruntime.WAITING)

	if err := booter.initTransports(); err != nil {
		return err
	}

	if err := booter.initMarket(); err != nil {
		return err
	}

	if err := booter.initPipeline(); err != nil {
		return err
	}

	if err := booter.seedBooks(); err != nil {
		return err
	}

	if err := booter.openFirehose(); err != nil {
		return err
	}

	booter.Transition(nmruntime.READY)

	errnie.Info(fmt.Sprintf(
		"[booter] trade firehose open %s after boot",
		time.Since(started).Round(time.Millisecond),
	))

	// Run is the daemon's lifetime: ingress runs in goroutines, so returning
	// here would let the command return, cancel the context, and exit.
	<-booter.Context().Done()
	store.FlushAll()
	return nil
}

func (booter *Booter) initTransports() error {
	ctx := booter.Context()

	booter.public = network.NewWebsocketClient(ctx)

	if err := booter.public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
		return errnie.Err(
			errnie.IO, "booter: public websocket open failed", err,
		)
	}

	switch system.Cfg.Market.Model {
	case "paper":
		booter.privateTransport = broker.NewPaper(ctx)
	case "real":
		privateWS := network.NewWebsocketClient(ctx)

		if err := privateWS.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
			return errnie.Err(
				errnie.IO, "booter: private websocket open failed", err,
			)
		}

		booter.privateTransport = privateWS
	default:
		return errnie.Err(
			errnie.Validation,
			"booter: market.model must be paper or real",
			nil,
		)
	}

	return nil
}

func (booter *Booter) initMarket() error {
	ctx := booter.Context()

	booter.instrument = broker.NewInstrument(ctx, booter.public)

	if err := booter.instrument.Error(); err != nil {
		return errnie.Err(
			errnie.IO,
			"booter: instrument registry failed during construction",
			err,
		)
	}

	booter.normalizer = broker.NormalizerFor(booter.instrument)
	booter.book = broker.NewBook(ctx, booter.normalizer)

	booter.balance = broker.NewBalance(
		ctx,
		booter.privateTransport,
		decimal.NewFromInt64(int64(system.Cfg.Market.Balance)),
	)

	booter.price = broker.NewPrice(
		ctx,
		booter.book,
		booter.privateTransport,
		booter.instrument,
		booter.normalizer,
		booter.balance,
	)

	if booter.price.Status() != nmruntime.READY {
		return errnie.Err(
			errnie.NotAcceptable,
			"[booter] initial price fees are not ready",
			nil,
		)
	}

	booter.desk = broker.NewDesk(
		ctx, booter.privateTransport,
		booter.price,
		booter.balance,
	)

	ui.UI.RegisterCallback(booter.desk.Hello)
	ui.UI.Exiter(hubDeskExiter{desk: booter.desk})
	booter.startPrivateIngress()

	if err := broker.SubscribeExecutions(ctx, booter.privateTransport, booter.instrument); err != nil {
		return err
	}

	if ws, ok := booter.privateTransport.(*network.WebsocketClient); ok {
		ws.OnReconnect(func() error {
			return broker.SubscribeExecutions(booter.Context(), ws, booter.instrument)
		})
	}

	return nil
}

func (booter *Booter) initPipeline() error {
	ctx := booter.Context()

	booter.signals = []nmruntime.Node{
		correlation.NewSignal(ctx),
		cvd.NewSignal(ctx, booter.book),
		depthflow.NewSignal(ctx, booter.book),
		hawkes.NewSignal(ctx),
		leadlag.NewSignal(ctx),
		liquidity.NewSignal(ctx, booter.book, booter.price),
		morphology.NewSignal(ctx, booter.book),
		pumpdump.NewSignal(ctx, booter.book),
		sentiment.NewSignal(ctx),
		toxicity.NewSignal(ctx, booter.book),
	}

	for _, node := range booter.signals {
		if node.Status() == nmruntime.ERROR || node.Status() == nmruntime.FATAL {
			return errnie.Err(
				errnie.Internal,
				fmt.Sprintf("booter: %s failed construction", node.Name()),
				node.Error(),
			)
		}
	}

	stage := strategy.NewStage(ctx, booter.desk)
	stage.Transition(nmruntime.READY)

	solver := resonance.NewSolver(ctx)
	trainingNodes := []nmruntime.Node{stage, solver}

	strategyNodes := make([]nmruntime.Node, 0, 1)

	// paper and real both run the strategy. real posts live market entries
	// on the private websocket. Any other model does not place orders.
	if system.Cfg.Market.Model == "paper" || system.Cfg.Market.Model == "real" {
		booter.training = strategy.NewTraining(
			ctx, booter.price, booter.desk,
		)

		if err := booter.training.Error(); err != nil {
			return err
		}

		ui.UI.RegisterCallback(func() {
			ui.UI.ListPrefix("model/paths/")
		})
		booter.training.Transition(nmruntime.READY)
		strategyNodes = append(strategyNodes, booter.training)
	}

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[]nmruntime.Ring{
			{Group: "signal", Nodes: booter.signals},
			{Group: "training", Nodes: trainingNodes},
			{Group: "strategy", Nodes: strategyNodes},
		},
	)
	for _, signal := range booter.signals {
		signal.Transition(nmruntime.READY)
	}

	booter.workspace.Transition(nmruntime.READY)
	return nil
}

/*
seedBooks is the level3 phase: every subscribed symbol's snapshot must be
applied (book READY) before any trade reaches the workspace. A symbol that does
not seed within market.l3_snapshot_timeout of the last subscribe fails the boot
with the venue's reply for it; the universe is never silently reduced.
*/
func (booter *Booter) seedBooks() error {
	timeout := viper.GetDuration("market.l3_snapshot_timeout")

	if timeout <= 0 {
		return errnie.Err(
			errnie.Validation,
			"[booter] market.l3_snapshot_timeout is not configured",
			nil,
		)
	}

	// The public socket carries acknowledgements now and trades only once
	// the firehose is opened.
	booter.startIngress(booter.public, "public")

	// Every socket is read while it subscribes: Kraken rejects Level3
	// snapshot requests on a socket whose earlier snapshots sit unread.
	booter.book.Expect(booter.instrument.Symbols())

	booter.book.SetResubscribe(func(symbol string) {
		if err := booter.instrument.ResubscribeLevel3(symbol); err != nil {
			errnie.Error(err)
		}
	})

	if err := booter.instrument.SubscribeBooks(broker.Level3{
		Stale: booter.book.Stale,
		Reader: func(client *network.WebsocketClient) {
			booter.startIngress(client, "level3")
		},
	}); err != nil {
		return errnie.Err(
			errnie.Internal,
			"[booter] level3 subscribe to instrument universe",
			err,
		)
	}

	if err := booter.book.Wait(timeout); err != nil {
		replies := make([]string, 0)

		for _, symbol := range booter.book.Pending() {
			reply := "no venue reply"

			if raw, ok := booter.replies.Load(symbol); ok {
				reply = raw.(string)
			}

			replies = append(replies, symbol+": "+reply)
		}

		return errnie.Err(
			errnie.NotFound,
			"[booter] level3 seeding failed; venue replies: "+strings.Join(replies, "; "),
			err,
		)
	}

	return nil
}

/*
startPrivateIngress reads the paper desk's private frames: executions are
applied to the desk and accounted by training; a rejected order is an
error. The paper transport blocks until each frame is read, so this runs
for as long as orders can be placed.
*/
func (booter *Booter) startPrivateIngress() {
	go func() {
		for {
			buf, err := booter.privateTransport.Read()

			if err != nil {
				if nmruntime.ShuttingDown(booter.Context(), err) || booter.Context().Err() != nil {
					return
				}

				if disconnect.Expected(err) {
					errnie.Warn("[booter] private ingress disconnected, retrying read")
				} else {
					errnie.Error(errnie.Err(
						errnie.IO, "[booter] private ingress read failed", err,
					))
				}

				continue
			}

			switch {
			case data.Attribute[string](buf, "channel") == "executions":
				execution, err := kraken.NewExecution(buf)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[booter] executions frame: "+string(buf),
						err,
					))

					continue
				}

				booter.desk.Apply(execution)
			case data.Attribute[string](buf, "method") == "add_order":
				if success, err := data.AttributeOk[bool](buf, "success"); err != nil || !success {
					errnie.Error(errnie.Err(
						errnie.IO,
						"[booter] paper desk rejected an order: "+string(buf),
						err,
					))
				}
			}
		}
	}()
}

/*
openFirehose is the last phase: the workspace and every book are READY, so the
first trade tick already carries a seeded book for each symbol it touches.
*/
func (booter *Booter) openFirehose() error {
	if err := booter.instrument.SubscribeTrades(); err != nil {
		return errnie.Err(
			errnie.Internal, "[booter] trade subscribe to instrument universe", err,
		)
	}

	if booter.instrument.Status() != nmruntime.READY {
		return errnie.Err(
			errnie.NotAcceptable, "[booter] instrument universe is not seeded", nil,
		)
	}

	return nil
}

func (booter *Booter) startIngress(
	client broker.Transport, name string,
) {
	go func() {
		errnie.Info(fmt.Sprintf("[booter] starting %s ingress", name))

		for {
			select {
			case <-booter.Context().Done():
				return
			default:
			}

			buf, err := client.Read()

			if err != nil {
				if disconnect.Expected(err) {
					errnie.Warn("[booter] ingress disconnected, retrying read")
				} else {
					booter.Error(errnie.Err(
						errnie.IO,
						"[booter] ingress read failed",
						err,
					))
				}

				continue
			}

			channel := data.Attribute[string](buf, "channel")
			msgType := data.Attribute[string](buf, "type")

			switch channel {
			case "level3":
				level3 := kraken.NewLevel3(buf)

				if level3 == nil {
					continue
				}

				if err := booter.book.Update(
					level3,
				); err != nil && !nmruntime.ShuttingDown(
					booter.Context(), err,
				) {
					errnie.Error(err)
				}

				// Reprice open positions whose book just changed so the stoploss
				// sees the new exit VWAP and depth without waiting for a trade.
				if booter.desk != nil {
					symbols := make([]string, 0, len(level3.Data))
					for _, d := range level3.Data {
						if d.Symbol != "" {
							symbols = append(symbols, d.Symbol)
						}
					}
					booter.desk.Refresh(symbols)
				}
			case "trade":
				data.Tick.Add(1)

				if booter.workspace == nil {
					continue
				}

				trade, err := kraken.NewTrade(buf)

				if err != nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[booter] trade frame does not decode",
						err,
					))

					continue
				}

				if booter.desk != nil {
					booter.desk.Update(trade.Data)
				}

				// Store the trade array, not the Kraken envelope
				// ({"channel","type","data"}). Signal readers decode
				// this layer as []kraken.TradeData.
				value := data.NewValue("workspace", "tick", "aggregate")
				value.Append(trade.Data, "spot:trade", msgType)

				// Step blocks until signals, the sequencer, and the model
				// have written this tick. The frame copied below is that result.
				out := booter.workspace.Step(value)

				io.Copy(ui.UI, out)
				if booter.store != nil {
					snapshot := out.Bytes()
					prefix := out.Prefix
					booter.store.WriteAtPrefix(prefix, snapshot)
				}
			case "heartbeat", "status":
			default:
				// Request acknowledgements (subscribe, unsubscribe, ...). A
				// rejection is an error; it is never dropped.
				if success, err := data.AttributeOk[bool](buf, "success"); err == nil && !success {
					errnie.Error(errnie.Err(
						errnie.IO,
						fmt.Sprintf("[booter] %s venue rejected %s %s: %s",
							name,
							data.Attribute[string](buf, "method"),
							data.Attribute[string](buf, "symbol"),
							data.Attribute[string](buf, "error"),
						),
						nil,
					))

					if name == "level3" {
						booter.replies.Store(
							data.Attribute[string](buf, "symbol"),
							"rejected: "+string(buf),
						)
					}

					continue
				}

				// Level3 replies are kept per symbol: a symbol that fails to
				// seed is reported with the venue's answer for it.
				if data.Attribute[string](buf, "result", "channel") == "level3" {
					booter.replies.Store(data.Attribute[string](
						buf, "result", "symbol",
					), "accepted: "+string(buf))
				}
			}
		}
	}()
}
