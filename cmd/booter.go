package cmd

import (
	"context"
	"fmt"
	sysruntime "runtime"
	"sync"
	"time"

	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/market"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/disconnect"
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
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
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
	persist    *tickPersist
	store      *store.Client
	replies    sync.Map
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

	network.SetPrivateSigner(kraken.SignPrivate)

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

	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	return &Booter{
		System: nmruntime.NewSystem(ctx, name, nil),
		store:  client,
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
	booter.persist.Arm()

	errnie.Info(fmt.Sprintf(
		"[booter] trade firehose open %s after boot",
		time.Since(started).Round(time.Millisecond),
	))

	// Run is the daemon's lifetime: ingress runs in goroutines, so returning
	// here would let the command return, cancel the context, and exit.
	<-booter.Context().Done()
	booter.store.Close()
	return nil
}

func (booter *Booter) initTransports() error {
	ctx := booter.Context()

	booter.virtual = network.NewVirtual(ctx)
	booter.paper = kraken.NewPaper(ctx)
	booter.public = network.NewWebsocketClient(ctx)
	booter.private = network.NewWebsocketClient(ctx)

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
	booter.book = broker.NewBookManager()

	booter.price = broker.NewPrice(
		ctx,
		booter.book,
		booter.instrument,
		booter.normalizer,
	)

	if booter.price.Status() != nmruntime.READY {
		return errnie.Err(
			errnie.NotAcceptable,
			"[booter] initial price fees are not ready",
			nil,
		)
	}

	booter.virtual.UseFills(booter.price)

	booter.desk = broker.NewDesk(
		ctx,
		map[string]network.Client{
			"virtual": booter.virtual,
			"paper":   booter.paper,
			"public":  booter.public,
			"private": booter.private,
		},
		booter.price,
	)

	ui.UI.RegisterCallback(func() {
		booter.desk.Hello()
	})

	return nil
}

func (booter *Booter) initPipeline() error {
	ctx := booter.Context()

	training := strategy.NewTraining(ctx, booter.price, booter.desk)
	model := strategy.NewModel(ctx, booter.desk, booter.price, training.ResetSequencer)
	booter.model = model
	booter.persist = newTickPersist(ctx, booter.store)

	booter.workspace = nmruntime.NewWorkspace(
		ctx,
		"workspace",
		[]nmruntime.Ring{
			{Group: "signal", Nodes: []nmruntime.Node{
				correlation.NewSignal(ctx),
				cvd.NewSignal(ctx, booter.book),
				depthflow.NewSignal(ctx, booter.book, booter.instrument),
				hawkes.NewSignal(ctx),
				leadlag.NewSignal(ctx),
				liquidity.NewSignal(ctx, booter.book),
				morphology.NewSignal(ctx, booter.book),
				pumpdump.NewSignal(ctx, booter.book),
				sentiment.NewSignal(ctx),
				toxicity.NewSignal(ctx, booter.book),
			}},
			{Group: "observe", Nodes: []nmruntime.Node{
				resonance.NewSolver(ctx),
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

	booter.desk.OnTrade(func(trade *market.Trade) {
		if booter.workspace == nil || trade == nil {
			return
		}

		tick := nmruntime.NewEnvelope(trade)
		tick.N = data.Tick.Add(1)

		ui.UI.WriteJSON(map[string]any{
			"tick": map[string]any{
				"epoch": data.Epoch.Load(),
				"n":     tick.N,
			},
		})

		booter.workspace.Step(tick)
	})

	booter.desk.Start()
	return nil
}

/*
seedBooks opens level3, waits until every symbol has a book, then continues.
*/
func (booter *Booter) seedBooks() error {
	symbols := booter.instrument.Symbols()
	booter.book.Expect(symbols)

	if err := booter.instrument.SubscribeBooks(broker.Level3{
		Stale: booter.book.Stale,
		Reader: func(client *network.WebsocketClient) {
			go booter.readLevel3(client)
		},
	}); err != nil {
		return errnie.Err(errnie.IO, "[booter] level3 subscribe failed", err)
	}

	timeout := 2 * time.Minute

	if err := booter.book.Wait(timeout); err != nil {
		return errnie.Err(errnie.IO, "[booter] level3 seed timed out", err)
	}

	return nil
}

func (booter *Booter) readLevel3(client *network.WebsocketClient) {
	for {
		select {
		case <-booter.Context().Done():
			return
		default:
		}

		buf, err := client.Read()

		if err != nil {
			if nmruntime.ShuttingDown(booter.Context(), err) || booter.Context().Err() != nil {
				return
			}

			if disconnect.Expected(err) {
				errnie.Warn("[booter] level3 disconnected, retrying read")
			} else {
				errnie.Error(errnie.Err(
					errnie.IO, "[booter] level3 ingress read failed", err,
				))
			}

			continue
		}

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
