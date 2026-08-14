package trader

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type streamConfig struct {
	symbolShards       int
	laneCapacity       int
	spinLimit          int
	drainLimit         int
	diagnosticInterval time.Duration
}

func newStreamConfig() (streamConfig, error) {
	config := streamConfig{
		symbolShards:       viper.GetInt("system.streaming.symbol_shards"),
		laneCapacity:       viper.GetInt("system.streaming.lane_capacity"),
		spinLimit:          viper.GetInt("system.streaming.spin_limit"),
		drainLimit:         viper.GetInt("system.streaming.drain_limit"),
		diagnosticInterval: viper.GetDuration("system.bus.heartbeat"),
	}

	if config.symbolShards <= 0 || config.symbolShards&(config.symbolShards-1) != 0 {
		return streamConfig{}, fmt.Errorf("stream: power-of-two symbol shard count required")
	}

	if config.laneCapacity <= 0 || config.laneCapacity&(config.laneCapacity-1) != 0 {
		return streamConfig{}, fmt.Errorf("stream: power-of-two lane capacity required")
	}

	if config.spinLimit < 0 || config.drainLimit <= 0 {
		return streamConfig{}, fmt.Errorf("stream: non-negative spin and positive drain limits required")
	}

	if config.diagnosticInterval <= 0 {
		return streamConfig{}, fmt.Errorf("stream: positive diagnostic interval required")
	}

	return config, nil
}

type marketEventKind uint8

const (
	marketEventTicker marketEventKind = iota
	marketEventTrade
	marketEventBook
	marketEventLevel3
)

type marketEvent struct {
	sequence       uint64
	tickerSequence uint64
	tick           int64
	parts          int
	kind           marketEventKind
	symbol         *types.Symbol
	at             time.Time
	ticker         kraken.TickerData
	trade          kraken.TradeData
	level3         kraken.Level3Data
	dispatchedAt   time.Time
	measuredAt     time.Time
	collectedAt    time.Time
}

/*
streamPipeline routes typed events into fixed affinity owners and commits their
results in ingress order. Only result commit touches global logic and planning;
market intake and every independent signal owner continue concurrently.
*/
type streamPipeline struct {
	ctx               context.Context
	cancel            context.CancelFunc
	config            streamConfig
	thesis            *types.Thesis
	analyzer          *logic.Analyzer
	planner           *strategy.Planner
	publisher         *Measurements
	workers           []*streamWorker
	local             []*streamWorker
	cross             []*streamWorker
	resultWake        chan struct{}
	commitInbox       *lane[*eventResults]
	commitWake        chan struct{}
	progress          chan struct{}
	committedSequence atomic.Uint64
	ingressSequence   atomic.Uint64
	tickerSequence    atomic.Uint64
	nextSequence      atomic.Uint64
	pendingCount      atomic.Int64
	lastCommitNanos   atomic.Int64
	tickers           atomic.Uint64
	books             atomic.Uint64
	trades            atomic.Uint64
	level3            atomic.Uint64
	coalescedBooks    atomic.Uint64
	droppedSequences  sync.Map
	bookQueued        sync.Map
	dropped           atomic.Uint64
	commitDropped     atomic.Uint64
	startedAt         time.Time
	lastBrokerAt      time.Time
	clocks            clockBank
	measurementsDirty map[string]bool
	resonanceDirty    map[string]bool
	analyzed          map[string]int64
	ui                chan []byte
	fail              func(error)
	wait              sync.WaitGroup
}

func newStreamPipeline(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	ui chan []byte,
	thesis *types.Thesis,
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
	fail func(error),
) (*streamPipeline, error) {
	config, err := newStreamConfig()

	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	pipeline := &streamPipeline{
		ctx:               ctx,
		cancel:            cancel,
		config:            config,
		thesis:            thesis,
		analyzer:          analyzer,
		planner:           planner,
		publisher:         newMeasurements(ctx, ui, nil),
		resultWake:        make(chan struct{}, 1),
		progress:          make(chan struct{}, 1),
		measurementsDirty: make(map[string]bool),
		resonanceDirty:    make(map[string]bool),
		analyzed:          make(map[string]int64),
		ui:                ui,
		fail:              fail,
		startedAt:         time.Now(),
	}
	pipeline.lastCommitNanos.Store(pipeline.startedAt.UnixNano())
	pipeline.nextSequence.Store(1)
	pipeline.bindModuleClocks(analyzer, planner)
	pipeline.commitWake = make(chan struct{}, 1)
	pipeline.commitInbox, err = newLane[*eventResults](
		config.laneCapacity,
		config.spinLimit,
		pipeline.commitWake,
	)

	if err != nil {
		pipeline.Close()
		return nil, err
	}

	for range config.symbolShards {
		measurements := newLocalMeasurements(ctx, api, instrument, ui)
		worker, err := pipeline.newWorker(measurements, true, nil)

		if err != nil {
			pipeline.Close()
			return nil, err
		}

		pipeline.local = append(pipeline.local, worker)
		pipeline.workers = append(pipeline.workers, worker)
	}

	for _, source := range []types.SourceType{
		types.SourceCorrelation,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourceSentiment,
	} {
		measurements := newCrossMeasurements(ctx, api, ui, source)
		worker, err := pipeline.newWorker(measurements, false, []types.SourceType{source})

		if err != nil {
			pipeline.Close()
			return nil, err
		}

		pipeline.cross = append(pipeline.cross, worker)
		pipeline.workers = append(pipeline.workers, worker)
	}

	pipeline.start()

	return pipeline, nil
}

func (pipeline *streamPipeline) Wait(
	ctx context.Context,
	sequence uint64,
) error {
	for {
		if pipeline.committedSequence.Load() >= sequence {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pipeline.ctx.Done():
			return fmt.Errorf(
				"stream: stopped before ingress sequence %d was committed",
				sequence,
			)
		case <-pipeline.progress:
		}
	}
}

func newLocalMeasurements(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	ui chan []byte,
) *Measurements {
	quotes := types.NewQuoteHistory(system.Cfg.PumpDump.Capacity)

	return newMeasurements(ctx, ui, []types.Signal{
		cvd.NewSignalWithQuotes(ctx, api, ui, quotes),
		depthflow.NewSignal(ctx, api, instrument, ui),
		exhaust.NewSignal(ctx, api, instrument, ui),
		hawkes.NewSignal(ctx, api, ui),
		pumpdump.NewSignalWithQuotes(ctx, api, ui, quotes),
		toxicity.NewSignal(ctx, api, ui),
	})
}

func newCrossMeasurements(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	source types.SourceType,
) *Measurements {
	var signal types.Signal

	switch source {
	case types.SourceCorrelation:
		signal = correlation.NewSignal(ctx, api, ui)
	case types.SourceLeadLag:
		signal = leadlag.NewSignal(ctx, api, ui)
	case types.SourceLiquidity:
		signal = liquidity.NewSignal(ctx, api, ui)
	case types.SourceSentiment:
		signal = sentiment.NewSignal(ctx, api, ui)
	}

	return newMeasurements(ctx, ui, []types.Signal{signal})
}

func (pipeline *streamPipeline) bindModuleClocks(
	analyzer *logic.Analyzer,
	planner *strategy.Planner,
) {
	if analyzer != nil {
		analyzer.ObserveModule = pipeline.clocks.observe
		analyzer.ObserveHop = pipeline.clocks.observeHop
	}

	if planner != nil {
		planner.ObserveModule = pipeline.clocks.observe
		planner.ObserveHop = pipeline.clocks.observeHop
	}
}

func (pipeline *streamPipeline) start() {
	for _, worker := range pipeline.workers {
		pipeline.wait.Add(1)
		go worker.run(&pipeline.wait)
	}

	pipeline.wait.Add(3)
	go pipeline.collect(&pipeline.wait)
	go pipeline.commit(&pipeline.wait)
	go pipeline.publishDiagnostics(&pipeline.wait)
}

func (pipeline *streamPipeline) Close() error {
	if pipeline == nil {
		return nil
	}

	pipeline.cancel()
	pipeline.wait.Wait()

	for _, worker := range pipeline.workers {
		if err := worker.measurements.Close(); err != nil {
			return err
		}
	}

	return pipeline.publisher.Close()
}
