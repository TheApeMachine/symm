package cmd

import (
	"context"
	"fmt"
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
	"github.com/theapemachine/symm/system"
)

/*
Booter encapsulates the bootstrapping, component assembly, and supervision
lifecycle for daemon processes (such as symm collect).
*/
type Booter struct {
	*nmruntime.System
	epoch            int64
	capture          bool
	public           *network.WebsocketClient
	privateTransport broker.Transport
	instrument       *broker.Instrument
	book             *broker.Book
	price            *broker.Price
	normalizer       *spot.Normalizer
	signals          []nmruntime.Node
	workspace        *nmruntime.Workspace
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

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[][]nmruntime.Node{booter.signals},
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
	// booter.instrument.SetLevel3Stale(booter.book.Stale)
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

				booter.book.Update(level3)
			case "trade":
				data.Tick.Add(1)
				if booter.workspace != nil {
					booter.workspace.Step(data.NewValue(
						"workspace", "tick", "aggregate",
					).AppendJSON(buf, "spot:trade/"+msgType))
				}
			}
		}
	}()
}
