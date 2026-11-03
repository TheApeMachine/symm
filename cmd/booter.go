package cmd

import (
	"context"
	"fmt"
	sysruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/grafana/pyroscope-go"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	shopdecimal "github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
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
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Booter encapsulates the bootstrapping, component assembly, and supervision
lifecycle for daemon processes (such as symm collect).
*/
type Booter struct {
	*nmruntime.System
	epoch            int64
	public           *network.WebsocketClient
	privateTransport network.Client
	liveTransport    network.Client
	instrument       *broker.Instrument
	book             *broker.Book
	price            *broker.Price
	desk             *broker.Desk
	normalizer       *spot.Normalizer
	signals          []nmruntime.Node
	training         *strategy.Training
	model            *strategy.Model
	workspace        *nmruntime.Workspace
	persist          *tickPersist
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

	booter.public = network.NewWebsocketClient(ctx)

	if err := booter.public.Open(system.Cfg.WebSocket.Endpoints.Public); err != nil {
		return errnie.Err(
			errnie.IO, "booter: public websocket open failed", err,
		)
	}

	// Paper transport is always present so the paper book can refine while
	// real is armed or halted.
	booter.privateTransport = kraken.NewPaper(ctx, venue.NewSimulator())

	switch system.Cfg.Market.Model {
	case "paper":
		// Real ceiling closed — live transport stays nil.
	case "real":
		privateWS := network.NewWebsocketClient(ctx)

		if err := privateWS.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
			return errnie.Err(
				errnie.IO, "booter: private websocket open failed", err,
			)
		}

		booter.liveTransport = kraken.NewLive(ctx, privateWS, nil)
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

	booter.desk = broker.NewDesk(
		ctx,
		map[string]network.Client{
			"virtual": network.NewVirtual(ctx),
			"paper":   booter.privateTransport,
			"real":    booter.liveTransport,
		},
		booter.price,
	)

	ui.UI.RegisterCallback(func() {
		booter.desk.Hello()
	})

	booter.startPrivateIngress()

	for _, channel := range []string{"executions", "balances"} {
		buf, err := sonic.Marshal(map[string]any{
			"method": "subscribe",
			"params": map[string]any{"channel": channel},
		})

		if err != nil {
			return errnie.Err(errnie.IO, "[booter] paper "+channel+" subscribe marshal failed", err)
		}

		if err := booter.privateTransport.Write(buf); err != nil {
			return errnie.Err(errnie.IO, "[booter] paper "+channel+" subscribe write failed", err)
		}
	}

	if live, ok := booter.liveTransport.(*kraken.Live); ok {
		booter.startLiveIngress()

		if err := live.SubscribeExecutions(); err != nil {
			return err
		}

		if err := live.SubscribeBalances(); err != nil {
			return err
		}

		live.OnReconnect(func() error {
			if err := live.SubscribeExecutions(); err != nil {
				return err
			}

			return live.SubscribeBalances()
		})
	}

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
	booter.pumpPrivate(booter.privateTransport, "paper")
}

func (booter *Booter) startLiveIngress() {
	booter.pumpPrivate(booter.liveTransport, "real")
}

func (booter *Booter) pumpPrivate(transport network.Client, label string) {
	if transport == nil {
		return
	}

	go func() {
		for {
			buf, err := transport.Read()

			if err != nil {
				if nmruntime.ShuttingDown(booter.Context(), err) || booter.Context().Err() != nil {
					return
				}

				if disconnect.Expected(err) {
					errnie.Warn("[booter] " + label + " ingress disconnected, retrying read")
				} else {
					errnie.Error(errnie.Err(
						errnie.IO, "[booter] "+label+" ingress read failed", err,
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
						"[booter] "+label+" executions frame: "+string(buf),
						err,
					))

					continue
				}

				booter.desk.Apply(execution)
			case data.Attribute[string](buf, "method") == "add_order":
				if success, err := data.AttributeOk[bool](buf, "success"); err != nil || !success {
					errnie.Error(errnie.Err(
						errnie.IO,
						"[booter] "+label+" desk rejected an order: "+string(buf),
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
	client network.Client, name string,
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

				// Reprice open lots after books are live. Doing this during
				// seed stalls the L3 reader (unread snapshots → venue rejects).
				if booter.desk != nil && booter.Status() == nmruntime.READY {
					symbols := make([]string, 0, len(level3.Data))

					for _, d := range level3.Data {
						if d.Symbol != "" {
							symbols = append(symbols, d.Symbol)
						}
					}

					booter.desk.Refresh(symbols)
				}

				// // Never block L3 Read on S3 while books are still seeding —
				// // unread snapshots make the venue reject later subscribes.
				// if booter.Status() == nmruntime.READY {
				// 	booter.store.WriteAtPrefix(
				// 		fmt.Sprintf(
				// 			"workspace/tick/level3/%019d/%019d",
				// 			data.Tick.Load(),
				// 			data.SeqIdx.Add(1),
				// 		),
				// 		buf,
				// 	)
				// }
			case "trade":
				tickN := data.Tick.Add(1)

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

				dat := make([]*market.TradeData, 0, len(trade.Data))

				for _, d := range trade.Data {
					price, err := shopdecimal.NewFromString(d.Price.String())

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							"[booter] trade frame: "+string(buf),
							err,
						))
					}

					dat = append(dat, &market.TradeData{
						Symbol:    d.Symbol,
						Side:      market.Side(d.Side),
						Price:     price,
						Qty:       shopdecimal.NewFromFloat(d.Qty),
						Timestamp: d.Timestamp,
					})
				}

				tick := runtime.NewEnvelope(market.NewTrade(
					trade.Channel,
					trade.Type,
					dat,
				))
				tick.N = tickN

				// Step returns immediately; the disruptor mutates this
				// envelope. Persist runs in the last ring — never marshal here.
				booter.workspace.Step(tick)

				if booter.Status() != nmruntime.READY {
					continue
				}

				ui.UI.WriteJSON(map[string]any{
					"tick": map[string]any{
						"epoch": data.Epoch.Load(),
						"n":     tickN,
					},
				})
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

/*
tickPersist writes the finished envelope after the model ring. Ingress must not
marshal that envelope — Step hands it to the disruptor asynchronously.
*/
type tickPersist struct {
	*nmruntime.System
	store *store.Client
	armed atomic.Bool
}

func newTickPersist(ctx context.Context, client *store.Client) *tickPersist {
	persist := &tickPersist{store: client}
	persist.System = nmruntime.NewSystem(ctx, "tick-persist", persist)
	persist.Transition(nmruntime.READY)
	return persist
}

func (persist *tickPersist) Arm() {
	persist.armed.Store(true)
}

func (persist *tickPersist) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	if envelope == nil || persist.store == nil || !persist.armed.Load() {
		return envelope
	}

	// Pipeline rings are done with this envelope. Marshal off-thread so JSON/S3
	// never stalls the disruptor (that freeze left impulse/kernels on last paint).
	go persist.write(envelope)
	return envelope
}

func (persist *tickPersist) write(envelope *nmruntime.Envelope) {
	payload, err := nmruntime.Sonic.Marshal(envelope)
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[tick-persist] marshal failed",
			err,
		).With("n", envelope.N))
		return
	}

	if _, err := persist.store.WriteAtPrefix(
		fmt.Sprintf("workspace/tick/aggregate/%019d", envelope.N),
		payload,
	); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[tick-persist] write failed",
			err,
		).With("n", envelope.N))
	}
}
