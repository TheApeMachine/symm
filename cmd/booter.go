package cmd

import (
	"context"
	"fmt"
	"io"
	sysruntime "runtime"

	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Booter encapsulates the bootstrapping, component assembly, and supervision
lifecycle for daemon processes (such as symm collect).
*/
type Booter struct {
	*nmruntime.System
	epoch            int64
	capture          bool
	dashboard        bool
	public           *network.WebsocketClient
	privateTransport broker.Transport
	instrument       *broker.Instrument
	book             *broker.Book
	price            *broker.Price
	balance          *broker.Balance
	desk             *broker.Desk
	normalizer       *spot.Normalizer
	signals          []nmruntime.Node
	workspace        *nmruntime.Workspace
	store            *store.Client
	hub              *ui.Hub
}

func NewBooter(ctx context.Context, name string, capture ...bool) *Booter {
	errnie.Apply(&errnie.Config{
		Level: viper.GetString("system.log.level"),
	})

	// This process signs Kraken REST calls with its own role's key pair.
	if err := kraken.UseCredentials(kraken.RoleCollector); err != nil {
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

	data.Epoch.Store(uint64(processStartedAt.UnixNano()))
	enableCapture := name == "collect"

	if len(capture) > 0 {
		enableCapture = capture[0]
	}

	return &Booter{
		System:  nmruntime.NewSystem(ctx, name, nil),
		capture: enableCapture,
	}
}

/*
WithDashboard makes this booter serve the dashboard: it constructs the ui Hub
(127.0.0.1:8765/ws) and io.Copy's every tick to it. Only the command behind
`make run` sets it; every other command runs headless.
*/
func (booter *Booter) WithDashboard() *Booter {
	booter.dashboard = true
	return booter
}

func (booter *Booter) Run() error {
	errnie.Info(fmt.Sprintf(
		"[booter] symm boot started with %d CPUs", sysruntime.NumCPU(),
	))

	if err := booter.initTransports(); err != nil {
		return err
	}

	if err := booter.initMarket(); err != nil {
		return err
	}

	if err := booter.initPipeline(); err != nil {
		return err
	}

	if err := booter.subscribe(); err != nil {
		return err
	}

	booter.Transition(nmruntime.READY)

	// Run is the daemon's lifetime: ingress runs in goroutines, so returning
	// here would let the command return, cancel the context, and exit.
	<-booter.Context().Done()
	return nil
}

func (booter *Booter) initTransports() error {
	ctx := booter.Context()

	booter.public = network.NewWebsocketClient(ctx)

	if err := booter.public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "booter: public websocket open failed", err,
		))
	}

	if system.Cfg.Market.Model == "paper" {
		booter.privateTransport = broker.NewPaper(ctx)
	}

	if booter.privateTransport == nil {
		privateWS := network.NewWebsocketClient(ctx)

		if err := privateWS.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "booter: private websocket open failed", err,
			))
		}

		booter.privateTransport = privateWS
	}

	return nil
}

func (booter *Booter) initMarket() error {
	ctx := booter.Context()

	booter.instrument = broker.NewInstrument(ctx, booter.public)
	booter.normalizer = spot.NewNormalizer()

	if err := broker.SeedNormalizer(booter.normalizer); err != nil {
		return err
	}

	booter.book = broker.NewBook(ctx, booter.normalizer)

	booter.price = broker.NewPrice(
		ctx,
		booter.book,
		booter.privateTransport,
		booter.instrument,
		booter.normalizer,
	)

	if err := booter.instrument.Error(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"booter: instrument registry failed during construction",
			err,
		))
	}

	if err := booter.price.GetFees(booter.instrument.Symbols()); err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[booter] initial fees are not available",
			nil,
		))
	}

	if booter.price.Status() != nmruntime.READY {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[booter] initial price fees are not ready",
			nil,
		))
	}

	booter.balance = broker.NewBalance(ctx, booter.privateTransport)
	booter.desk = broker.NewDesk(
		ctx, booter.privateTransport, booter.price, booter.balance,
	)

	return nil
}

func (booter *Booter) initPipeline() error {
	ctx := booter.Context()

	booter.signals = []nmruntime.Node{
		correlation.NewSignal(ctx),
		cvd.NewSignal(ctx),
		depthflow.NewSignal(ctx),
		hawkes.NewSignal(ctx),
		leadlag.NewSignal(ctx),
		liquidity.NewSignal(ctx),
		morphology.NewSignal(ctx),
		pumpdump.NewSignal(ctx),
		sentiment.NewSignal(ctx),
		toxicity.NewSignal(ctx),
	}

	for _, node := range booter.signals {
		if node.Status() == nmruntime.ERROR || node.Status() == nmruntime.FATAL {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("booter: %s failed construction", node.Name()),
				node.Error(),
			))
		}
	}

	sinks := []io.Writer{}

	if booter.capture {
		booter.store = store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

		if booter.store == nil {
			return errnie.Error(errnie.Err(
				errnie.IO, "booter: store client unavailable for capture", nil,
			))
		}

		sinks = append(sinks, booter.store)
	}

	// Only the dashboard command io.Copy's every tick to the hub
	// (127.0.0.1:8765/ws); every other command is headless.
	if booter.dashboard {
		booter.hub = ui.NewHub(ctx)
		booter.hub.Run()
		sinks = append(sinks, booter.hub)
	}

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[][]nmruntime.Node{booter.signals},
		sinks...,
	)

	for _, signal := range booter.signals {
		signal.Transition(nmruntime.READY)
	}

	booter.workspace.Transition(nmruntime.READY)
	return nil
}

func (booter *Booter) subscribe() error {
	booter.startIngress(booter.public, "public")

	// Every socket is read while it subscribes: Kraken rejects Level3
	// snapshot requests on a socket whose earlier snapshots sit unread.
	booter.instrument.SetLevel3Stale(booter.book.Stale)
	booter.instrument.SetLevel3Reader(func(client *network.WebsocketClient) {
		booter.startIngress(client, "level3")
	})

	if err := booter.instrument.Subscribe(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"booter: subscribe to instrument universe",
			err,
		))
	}

	if booter.instrument.Status() != nmruntime.READY {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"booter: instrument universe is not seeded",
			nil,
		))
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
				booter.Error(errnie.Err(
					errnie.IO,
					"[booter] ingress read failed",
					err,
				))

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

				if err := booter.book.Update(level3); err != nil {
					errnie.Error(err)
				}
			case "trade":
				data.Tick.Add(1)

				if booter.workspace == nil {
					continue
				}

				// One Kraken frame is one tick (it may carry several symbols).
				// The tick's Value carries the state of every component it
				// touches, so a stored tick reproduces book, price, instrument,
				// wallet and positions for every symbol in the frame.
				value := data.NewValue(
					"workspace", "tick", "aggregate",
				).AppendJSON(buf, "spot:trade/"+msgType)

				trades, err := data.AttributeOk[[]kraken.TradeData](buf, "data")

				if err != nil {
					booter.Error(err)
					continue
				}

				for _, symbol := range broker.Symbols(trades) {
					value.AppendJSON(booter.book.JSON(symbol), "book/"+symbol).
						AppendJSON(booter.price.JSON(symbol), "price/"+symbol).
						AppendJSON(booter.price.InstrumentJSON(symbol), "instrument/"+symbol)
				}

				value.AppendJSON(booter.desk.BalanceJSON(), "balance").
					AppendJSON(booter.desk.PositionsJSON(), "positions")

				// Signals write one layer per trade entry (or per symbol),
				// each into its own pre-reserved slot.
				value.Reserve(len(booter.signals) * max(1, len(trades)))
				booter.workspace.Step(value)
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

					continue
				}

				errnie.Debug(fmt.Sprintf("[booter] %s frame %s %s %s", name,
					data.Attribute[string](buf, "method"),
					data.Attribute[string](buf, "result", "channel"),
					data.Attribute[string](buf, "result", "symbol"),
				))
			}
		}
	}()
}
