package cmd

import (
	"context"
	"errors"
	"fmt"
	sysruntime "runtime"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/market"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/pipeline"
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
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/strategy/deskgate"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Booter encapsulates the bootstrapping, component assembly, and supervision
lifecycle for daemon processes (such as symm collect).
*/
type Booter struct {
	*nmruntime.System
	epoch      int64
	public     network.Client
	private    network.Client
	virtual    *network.Virtual
	paper      network.Client
	instrument *broker.Instrument
	book       *broker.BookManager
	price      *broker.Price
	desk       *broker.Desk
	normalizer *spot.Normalizer
	signals    []nmruntime.Node
	training   *strategy.Training
	model      *strategy.Model
	workspace  *nmruntime.Workspace
	persist    *TickPersist
	graph      *pipeline.Graph
	store      *store.Client
	replies    sync.Map
	credErr    error
	live       bool
	engine     string
}

func NewBooter(ctx context.Context, name string) *Booter {
	errnie.Apply(&errnie.Config{
		Level: viper.GetString("system.log.level"),
	})

	// Paper signs with the collector key. Live orders use the main key so
	// they do not share a nonce stream with a collector process.
	role := kraken.RoleCollector
	live := system.Cfg.Market.Model == "real"

	if live {
		role = kraken.RoleMain
	}

	var credErr error

	if err := kraken.UseCredentials(role); err != nil {
		credErr = errnie.Error(errnie.Err(
			errnie.Validation, "[booter] kraken credentials unavailable", err,
		))

		// Live must not boot without RoleMain keys. Paper still logs and
		// continues — virtual/paper books do not require the private signer.
		if !live {
			errnie.Error(credErr)
			credErr = nil
		}
	}

	// appName := "symm." + name + ".theapemachine.app"

	// _, err := pyroscope.Start(pyroscope.Config{
	// 	ApplicationName: appName,
	// 	ServerAddress:   "http://localhost:4040",
	// 	Logger:          nil,
	// 	ProfileTypes: []pyroscope.ProfileType{
	// 		pyroscope.ProfileCPU,
	// 		pyroscope.ProfileAllocObjects,
	// 		pyroscope.ProfileAllocSpace,
	// 		pyroscope.ProfileInuseObjects,
	// 		pyroscope.ProfileInuseSpace,
	// 		pyroscope.ProfileGoroutines,
	// 		pyroscope.ProfileMutexCount,
	// 		pyroscope.ProfileMutexDuration,
	// 		pyroscope.ProfileBlockCount,
	// 		pyroscope.ProfileBlockDuration,
	// 	},
	// })

	// if err != nil {
	// 	errnie.Error(errnie.Err(
	// 		errnie.IO,
	// 		"[booter] error starting pyroscope profiler",
	// 		err,
	// 	))
	// } else {
	// 	// Enable mutex/block samplers only after the agent is up, at coarse
	// 	// rates so profiling does not dominate tick latency.
	// 	sysruntime.SetMutexProfileFraction(100)
	// 	sysruntime.SetBlockProfileRate(1_000_000)
	// 	errnie.Info("[booter] pyroscope profiling " + appName)
	// }

	client := store.NewClient(
		ctx,
		system.Cfg.Storage.S3.Bucket,
		system.Cfg.Storage.S3.Endpoint,
		system.Cfg.Storage.S3.Region,
	)

	if ui.UI != nil {
		ui.UI.UseStore(client)
	}

	return &Booter{
		System:  nmruntime.NewSystem(ctx, name, nil),
		store:   client,
		credErr: credErr,
		live:    live,
	}
}

func (booter *Booter) Run() error {
	if booter.credErr != nil {
		booter.setEngine("error")
		return booter.credErr
	}

	errnie.Info(fmt.Sprintf(
		"[booter] symm boot started with %d CPUs", sysruntime.NumCPU(),
	))

	started := time.Now()
	booter.setEngine("booting")

	// Transports and the market come first: training paper-trades through the
	// price and the desk. The workspace is up before any trade is subscribed.
	booter.Transition(nmruntime.WAITING)

	if err := booter.initTransports(); err != nil {
		booter.setEngine("error")
		return err
	}

	if err := booter.initMarket(); err != nil {
		booter.setEngine("error")
		return err
	}

	if err := booter.initPipeline(); err != nil {
		booter.setEngine("error")
		return err
	}

	if err := booter.seedBooks(); err != nil {
		booter.setEngine("error")
		return err
	}

	booter.desk.Start()

	if err := booter.openFirehose(); err != nil {
		booter.setEngine("error")
		return err
	}

	booter.Transition(nmruntime.READY)

	errnie.Info(fmt.Sprintf(
		"[booter] trade firehose open %s after boot",
		time.Since(started).Round(time.Millisecond),
	))

	booter.setEngine("ready")

	// Run is the daemon's lifetime: ingress runs in goroutines, so returning
	// here would let the command return, cancel the context, and exit.
	<-booter.Context().Done()

	return booter.shutdown()
}

/*
setEngine publishes booting / ready / error so the dashboard ONLINE badge
tracks firehose readiness, not only hub socket connect.
*/
func (booter *Booter) setEngine(status string) {
	booter.engine = status

	if ui.UI == nil {
		return
	}

	ui.UI.WriteJSON(map[string]any{
		"engine": status,
	})
}

/*
shutdown stops ingress acceptance, persists open state, drains S3, then
releases pools. Parent cancel already woke transports; store life survives
that cancel so Flush can finish uploads.
*/
func (booter *Booter) shutdown() error {
	errnie.Info("[booter] shutting down")

	var errs error

	if err := booter.closeTransports(); err != nil {
		errs = errors.Join(errs, err)
	}

	if booter.desk != nil {
		if err := booter.desk.Flush(); err != nil {
			errs = errors.Join(errs, errnie.Error(errnie.Err(
				errnie.IO, "[booter] desk flush failed", err,
			)))
		}
	}

	if booter.training != nil {
		if err := booter.training.Flush(); err != nil {
			errs = errors.Join(errs, errnie.Error(errnie.Err(
				errnie.IO, "[booter] training flush failed", err,
			)))
		}
	}

	if booter.store != nil {
		if err := booter.store.Close(); err != nil {
			errs = errors.Join(errs, errnie.Error(errnie.Err(
				errnie.IO, "[booter] store close failed", err,
			)))
		}
	}

	if err := ui.UI.Close(); err != nil {
		errs = errors.Join(errs, errnie.Error(errnie.Err(
			errnie.IO, "[booter] hub close failed", err,
		)))
	}

	if errs != nil {
		return errs
	}

	errnie.Info("[booter] shutdown complete")
	return nil
}

func (booter *Booter) closeTransports() error {
	var errs error

	closeOne := func(err error) {
		if err != nil {
			errs = errors.Join(errs, errnie.Error(errnie.Err(
				errnie.IO, "[booter] transport close failed", err,
			)))
		}
	}

	if booter.public != nil {
		if c, ok := booter.public.(interface{ Close() error }); ok {
			closeOne(c.Close())
		}
	}

	if booter.private != nil {
		if c, ok := booter.private.(interface{ Close() error }); ok {
			closeOne(c.Close())
		}
	}

	if booter.paper != nil {
		if c, ok := booter.paper.(interface{ Close() error }); ok {
			closeOne(c.Close())
		}
	}

	if booter.virtual != nil {
		closeOne(booter.virtual.Close())
	}

	return errs
}

func (booter *Booter) initTransports() error {
	ctx := booter.Context()

	// Live-only: no virtual wallet or paper CLI — they race the main key's
	// nonce stream and open shadow books we do not want armed.
	if !booter.live {
		booter.virtual = network.NewVirtual(ctx)
		booter.paper = kraken.NewPaper(ctx)
	}

	booter.public = network.NewWebsocketClient(ctx, system.Cfg.WebSocket.Endpoints.Public)
	booter.private = network.NewWebsocketClient(ctx, system.Cfg.WebSocket.Endpoints.Private)

	return nil
}

func (booter *Booter) initMarket() error {
	ctx := booter.Context()

	booter.normalizer = spot.NewNormalizer()

	if err := booter.normalizer.Use(spot.NewREST()); err != nil {
		return errnie.Err(
			errnie.IO,
			"[booter] Kraken public Assets/AssetPairs seed failed",
			err,
		)
	}

	booter.instrument = broker.NewInstrument(ctx, booter.public, booter.normalizer)

	if err := booter.instrument.Error(); err != nil {
		return errnie.Err(
			errnie.IO,
			"booter: instrument registry failed during construction",
			err,
		)
	}

	booter.book = broker.NewBookManager(booter.normalizer)

	booter.price = broker.NewPrice(
		ctx,
		booter.book,
		booter.instrument,
		booter.normalizer,
		venue.NewKraken(system.Cfg.Market.QuoteCurrency).Use(booter.normalizer),
		booter.public,
	)

	if booter.price.Status() != nmruntime.READY {
		return errnie.Err(
			errnie.NotAcceptable,
			"[booter] initial price fees are not ready",
			nil,
		)
	}

	transports := map[string]network.Client{
		"public":  booter.public,
		"private": booter.private,
	}

	if booter.virtual != nil {
		transports["virtual"] = booter.virtual
	}

	if booter.paper != nil {
		transports["paper"] = booter.paper
	}

	booter.desk = broker.NewDesk(ctx, transports, booter.price, booter.store)

	ui.UI.OnExit(func(book, id, symbol string) error {
		tt := broker.TierTypePaper

		switch book {
		case "virtual":
			tt = broker.TierTypeVirtual
		case "real":
			tt = broker.TierTypeReal
		}

		position := booter.desk.GetPosition(tt, symbol)

		if position == nil {
			return errnie.Error(errnie.Err(
				errnie.NotFound, "[booter] position not found", nil,
			))
		}

		return booter.desk.Exit(position, true)
	})

	ui.UI.RegisterCallback(func() {
		// Topology first so a busy Hello fan-out cannot crowd it out of the
		// hub queue on refresh.
		if booter.graph != nil {
			booter.graph.PublishTopology()
		}

		booter.desk.Hello()

		if booter.engine != "" {
			booter.setEngine(booter.engine)
		}
	})

	return nil
}

func (booter *Booter) initPipeline() error {
	ctx := booter.Context()

	deskGate := deskgate.NewGate(ctx, booter.store)

	training := strategy.NewTraining(
		ctx, booter.price, booter.desk, booter.store, deskGate,
	)

	model := strategy.NewModel(
		ctx, booter.desk, booter.price, training.ResetSequencer, booter.store,
	)

	booter.training = training
	booter.model = model
	booter.persist = NewTickPersist(ctx, booter.store)

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[]nmruntime.Ring{
			{Group: "signal", Nodes: []nmruntime.Node{
				correlation.NewSignal(ctx, booter.price),
				cvd.NewSignal(ctx, booter.book),
				depthflow.NewSignal(ctx, booter.book, booter.instrument),
				hawkes.NewSignal(ctx),
				leadlag.NewSignal(ctx),
				liquidity.NewSignal(ctx, booter.book),
				morphology.NewSignal(ctx, booter.book),
				pumpdump.NewSignal(ctx, booter.book, booter.price),
				sentiment.NewSignal(ctx),
				toxicity.NewSignal(ctx, booter.book),
			}},
			// Resonance then training are sequential rings: both mutate/read
			// envelope maps, and a shared handler group races Metrics().
			{Group: "resonance", Nodes: []nmruntime.Node{
				resonance.NewSolver(ctx),
			}},
			{Group: "training", Nodes: []nmruntime.Node{
				training,
			}},
			{Group: "model", Nodes: []nmruntime.Node{
				model,
			}},
			{Group: "persist", Nodes: []nmruntime.Node{
				booter.persist,
			}},
		},
	)

	booter.workspace.Transition(nmruntime.READY)
	// booter.graph.PublishTopology()
	// go booter.watchPipeline()

	booter.desk.OnTrade(func(trade *market.Trade) {
		if booter.workspace == nil || trade == nil {
			return
		}

		// started := time.Now()
		tick := nmruntime.NewEnvelope(trade)
		tick.N = data.Tick.Add(1)

		booter.workspace.Step(tick)
		// booter.graph.Ingress(time.Since(started))
	})

	return nil
}

/*
watchPipeline fails the engine badge when ingress keeps moving while model is
frozen. Tick count alone is not health — it publishes before the disruptor runs.
*/
func (booter *Booter) watchPipeline() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastIngress, lastModel uint64
	stalled := false

	for {
		select {
		case <-booter.Context().Done():
			return
		case <-ticker.C:
			if booter.graph == nil {
				continue
			}

			ingress := booter.graph.Count("ingress")
			model := booter.graph.Count("model")
			correlation := booter.graph.Count("correlation")

			if ingress >= lastIngress+50 && model == lastModel {
				errnie.Error(errnie.Err(
					errnie.Timeout,
					"[booter] pipeline stall: ingress advancing while model is frozen",
					nil,
				).With("ingress", ingress).
					With("model", model).
					With("correlation", correlation))

				if !stalled {
					booter.setEngine("error")
					stalled = true
				}
			} else if stalled && model > lastModel {
				booter.setEngine("ready")
				stalled = false
			}

			lastIngress = ingress
			lastModel = model
		}
	}
}

/*
seedBooks opens level3, waits until every symbol has a book, then continues.
*/
func (booter *Booter) seedBooks() error {
	symbols := booter.instrument.Symbols()
	booter.book.Expect(symbols)

	read := func(client *network.WebsocketClient) {
		go booter.readLevel3(client)
	}

	booter.book.OnInvalid(func(symbols []string) {
		booter.instrument.RecoverBooks(read, symbols, booter.book.Stale)
	})

	if err := booter.instrument.SubscribeBooks(read); err != nil {
		return errnie.Err(errnie.IO, "[booter] level3 subscribe failed", err)
	}

	const seedWait = 2 * time.Minute
	const seedRetry = 30 * time.Second

	for {
		if err := booter.book.Wait(seedWait); err != nil {
			if booter.Context().Err() != nil {
				return errnie.Err(errnie.IO, "[booter] level3 seed canceled", err)
			}

			errnie.Warn("[booter] level3 books still seeding, retrying: " + err.Error())

			select {
			case <-booter.Context().Done():
				return booter.Context().Err()
			case <-time.After(seedRetry):
			}

			continue
		}

		errnie.Info(fmt.Sprintf(
			"[booter] level3 books seeded for %d symbols",
			len(symbols),
		))

		return nil
	}
}

func (booter *Booter) readLevel3(client *network.WebsocketClient) {
	if client == nil {
		return
	}

	for {
		select {
		case <-booter.Context().Done():
			return
		case <-client.Context().Done():
			return
		default:
		}

		buf, err := client.Read()

		if err != nil {
			if nmruntime.ShuttingDown(booter.Context(), err) ||
				booter.Context().Err() != nil ||
				client.Context().Err() != nil {
				return
			}

			if !network.Recoverable(err) {
				errnie.Error(errnie.Err(
					errnie.IO, "[booter] level3 ingress read failed", err,
				))
			}

			continue
		}

		pipeline.Hit("level3")
		booter.desk.Feed(buf)
	}
}

/*
openFirehose is last: books and workspace are READY, then trades may flow.
*/
func (booter *Booter) openFirehose() error {
	if err := booter.instrument.SubscribeTrades(); err != nil {
		return errnie.Err(
			errnie.Internal, "[booter] trade subscribe failed", err,
		)
	}

	if booter.instrument.Status() != nmruntime.READY {
		return errnie.Err(
			errnie.NotAcceptable, "[booter] instrument universe is not seeded", nil,
		)
	}

	return nil
}
