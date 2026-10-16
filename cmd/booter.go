package cmd

import (
	"context"
	"errors"
	"fmt"
	sysruntime "runtime"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/hindsight"
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
pipelineSignal represents a signal node that is both a RuntimeSystem (with
lifecycle status) and a Node (executable in a Workspace Disruptor stage).
*/
type pipelineSignal interface {
	nmruntime.RuntimeSystem
	nmruntime.Node
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
	storeTee         *hindsight.StoreTee
	instrument       *broker.Instrument
	book             *broker.Book
	price            *broker.Price
	normalizer       *spot.Normalizer
	signals          []pipelineSignal
	workspace        *nmruntime.Workspace
	ingressHalted    chan error
}

func NewBooter(ctx context.Context, name string) *Booter {
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

	epoch := processStartedAt.UnixNano()
	system.Epoch.Store(epoch)

	return &Booter{
		System:        nmruntime.NewSystem(ctx, name, nil),
		epoch:         epoch,
		ingressHalted: make(chan error, 1),
	}
}

func (booter *Booter) Run() error {
	errnie.Info(fmt.Sprintf(
		"[booter] symm boot started with %d CPUs", sysruntime.NumCPU(),
	))

	if err := booter.initTransports(); err != nil {
		return err
	}

	if err := booter.initStorage(); err != nil {
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
	return booter.supervise()
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

func (booter *Booter) initStorage() error {
	booter.storeTee = hindsight.NewStoreTee(booter.Context(), "storeTee")
	booter.storeTee.Transition(nmruntime.READY)
	booter.storeTee.Run()

	booter.storeTee.Push(data.NewValue(
		"booter", "init", "s3",
	))

	return nil
}

func (booter *Booter) initMarket() error {
	ctx := booter.Context()

	booter.instrument = broker.NewInstrument(ctx, booter.public, booter.storeTee)
	booter.normalizer = spot.NewNormalizer()

	if err := broker.SeedNormalizer(booter.normalizer); err != nil {
		return err
	}

	booter.book = broker.NewBook(ctx, booter.normalizer)
	booter.book.SetResync(func(symbol string) {
		if resyncErr := booter.instrument.Resync(symbol); resyncErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[booter] book resync failed for "+symbol,
				resyncErr,
			))
		}
	})

	booter.price = broker.NewPrice(
		ctx, booter.book, booter.privateTransport, booter.instrument, booter.normalizer,
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

	booter.signals = []pipelineSignal{
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

	nodes := make([]nmruntime.Node, len(booter.signals))
	for i, s := range booter.signals {
		nodes[i] = s
	}

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[][]nmruntime.Node{nodes},
		booter.storeTee,
	)

	for _, s := range booter.signals {
		s.Transition(nmruntime.READY)
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
		defer func() {
			if r := recover(); r != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[booter] %s ingress recovered from panic: %v", name, r),
					nil,
				))
			}
		}()

		errnie.Info(fmt.Sprintf("[booter] starting %s ingress", name))

		halt := func(err error) {
			select {
			case booter.ingressHalted <- err:
			default:
			}
		}

		ctx := booter.Context()

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
						fmt.Sprintf("booter: %s ingress transport closed", name),
						errors.Join(closed.Error(), err),
					))
					return
				}

				continue
			}

			if rejection := subscribeRejection(buf); rejection != nil {
				softer, isSoft := client.(interface{ SoftDown(error) error })
				if isSoft && name == "level3" {
					errnie.Warn(fmt.Sprintf(
						"[booter] %s subscription rejected, backing off and redialing: %s",
						name, rejection,
					))

					time.Sleep(5 * time.Second)
					err := softer.SoftDown(rejection)
					errnie.Error(err)
					continue
				}

				halt(errnie.Err(
					errnie.NotAcceptable,
					fmt.Sprintf("booter: %s subscription rejected", name),
					rejection,
				))
				return
			}

			channel := data.Attribute[string](buf, "channel")
			if channel == "" {
				continue
			}

			switch channel {
			case "level3":
				level3, err := kraken.NewLevel3(buf)

				if err != nil {
					booter.Error(errnie.Err(
						errnie.NotAcceptable,
						"[boot] failed to parse level3 message",
						err,
					))
				}

				booter.book.Update(level3)
			case "trade":
				if booter.workspace != nil {
					booter.workspace.Step(data.NewValue(
						"workspace", "tick", "aggregate",
					).Append(buf))
				}
			}
		}
	}()
}

func (booter *Booter) supervise() error {
	ctx := booter.Context()

	haltWatched := make([]nmruntime.RuntimeSystem, 0, 1+len(booter.signals))
	haltWatched = append(haltWatched, booter.instrument)
	for _, s := range booter.signals {
		haltWatched = append(haltWatched, s)
	}

	nodeHalted := make(chan nmruntime.RuntimeSystem, len(haltWatched))

	for _, node := range haltWatched {
		go func(n nmruntime.RuntimeSystem) {
			select {
			case <-ctx.Done():
			case <-n.Context().Done():
				nodeHalted <- n
			}
		}(node)
	}

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			errnie.Info("[booter] shutdown requested, flushing in-flight measurements...")

			timeout := time.After(15 * time.Second)
			for booter.storeTee.Pending() > 0 {
				select {
				case <-timeout:
					errnie.Warn("[booter] storeTee flush timed out on shutdown")
					return nil
				case <-time.After(100 * time.Millisecond):
				}
			}

			errnie.Info("[booter] final storeTee flush complete")
			return nil

		case err := <-booter.ingressHalted:
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
				fmt.Sprintf("booter: %s halted", node.Name()),
				node.Error(),
			))
		}
	}

	return nil
}

func subscribeRejection(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	method := data.Attribute[string](buf, "method")
	if method != "subscribe" {
		return nil
	}
	success, hasSuccess := data.AttributeOk[bool](buf, "success")
	errMsg := data.Attribute[string](buf, "error")
	if (hasSuccess && !success) || errMsg != "" {
		if errMsg == "" {
			errMsg = "subscription rejected"
		}
		return errors.New(errMsg)
	}
	return nil
}
