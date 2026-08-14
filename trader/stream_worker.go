package trader

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

var eventResultsPool = sync.Pool{
	New: func() any {
		return &eventResults{
			measurements: make([]*types.Measurement, 0, 8),
		}
	},
}

type measurementResult struct {
	event        marketEvent
	measurements []*types.Measurement
	err          error
}

type streamWorker struct {
	ctx          context.Context
	name         string
	drainLimit   int
	local        bool
	receivers    []types.SourceType
	measurements *Measurements
	inbox        *lane[marketEvent]
	level3       *lane[marketEvent]
	outbox       *lane[measurementResult]
	bookQueued   *sync.Map
	clocks       *clockBank
	wake         chan struct{}
}

func (pipeline *streamPipeline) newWorker(
	measurements *Measurements,
	local bool,
	receivers []types.SourceType,
) (*streamWorker, error) {
	wake := make(chan struct{}, 1)
	inbox, err := newLane[marketEvent](
		pipeline.config.laneCapacity,
		pipeline.config.spinLimit,
		wake,
	)

	if err != nil {
		return nil, err
	}

	outbox, err := newLane[measurementResult](
		pipeline.config.laneCapacity,
		pipeline.config.spinLimit,
		pipeline.resultWake,
	)

	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("local-%d", len(pipeline.local))

	if !local && len(receivers) > 0 {
		name = string(receivers[0])
	}

	worker := &streamWorker{
		ctx:          pipeline.ctx,
		name:         name,
		drainLimit:   pipeline.config.drainLimit,
		local:        local,
		receivers:    receivers,
		measurements: measurements,
		inbox:        inbox,
		outbox:       outbox,
		clocks:       &pipeline.clocks,
		wake:         wake,
	}

	if local {
		worker.bookQueued = &pipeline.bookQueued
		level3, err := newLane[marketEvent](
			pipeline.config.laneCapacity,
			pipeline.config.spinLimit,
			wake,
		)

		if err != nil {
			return nil, err
		}

		worker.level3 = level3
	}

	if measurements != nil {
		measurements.clocks = &pipeline.clocks
	}

	return worker, nil
}

func (pipeline *streamPipeline) Dispatch(event marketEvent) error {
	if event.symbol == nil || event.sequence == 0 {
		return fmt.Errorf("stream: complete event identity required")
	}

	started := time.Now()
	event.dispatchedAt = started
	defer pipeline.clocks.observe("crypto", time.Since(started))
	pipeline.ingressSequence.Store(event.sequence)
	pipeline.noteKind(event.kind)

	if !pipeline.lastBrokerAt.IsZero() {
		pipeline.clocks.observeHop("price", "crypto", started.Sub(pipeline.lastBrokerAt))
		pipeline.lastBrokerAt = time.Time{}
	}

	localIndex := int(event.symbol.ID) & (len(pipeline.local) - 1)
	event.parts = 1

	if event.kind == marketEventTicker {
		event.tickerSequence = pipeline.tickerSequence.Add(1)
		event.parts += len(pipeline.cross)
	}

	if event.kind == marketEventBook {
		if _, loaded := pipeline.bookQueued.LoadOrStore(event.symbol.Symbol, struct{}{}); loaded {
			pipeline.coalescedBooks.Add(1)
			pipeline.markCommitted(event)
			return nil
		}
	}

	target := pipeline.local[localIndex].inbox

	if event.kind == marketEventLevel3 && pipeline.local[localIndex].level3 != nil {
		target = pipeline.local[localIndex].level3
	}

	if !target.TryPush(event) {
		pipeline.dropDispatch(event)
		return nil
	}

	if event.kind != marketEventTicker {
		return nil
	}

	for _, worker := range pipeline.cross {
		if err := worker.inbox.Push(pipeline.ctx, event); err != nil {
			return err
		}
	}

	return nil
}

func (pipeline *streamPipeline) dropDispatch(event marketEvent) {
	if event.kind == marketEventBook {
		pipeline.bookQueued.Delete(event.symbol.Symbol)
	}

	pipeline.dropped.Add(1)

	if event.kind == marketEventTicker {
		pipeline.droppedSequences.Store(event.tickerSequence, struct{}{})

		select {
		case pipeline.resultWake <- struct{}{}:
		default:
		}
	}

	pipeline.markCommitted(event)
}

func (pipeline *streamPipeline) noteKind(kind marketEventKind) {
	switch kind {
	case marketEventTicker:
		pipeline.tickers.Add(1)
	case marketEventTrade:
		pipeline.trades.Add(1)
	case marketEventBook:
		pipeline.books.Add(1)
	case marketEventLevel3:
		pipeline.level3.Add(1)
	}
}

func (worker *streamWorker) run(wait *sync.WaitGroup) {
	defer wait.Done()

	for {
		worked, stop := worker.drainLane(worker.inbox)

		if stop {
			return
		}

		if !worked && worker.level3 != nil {
			worked, stop = worker.drainLane(worker.level3)

			if stop {
				return
			}
		}

		if worked {
			continue
		}

		select {
		case <-worker.ctx.Done():
			return
		case <-worker.wake:
		}
	}
}

func (worker *streamWorker) drainLane(inbox *lane[marketEvent]) (bool, bool) {
	if inbox == nil {
		return false, false
	}

	worked := false

	for range worker.drainLimit {
		event, ok := inbox.Pop()

		if !ok {
			break
		}

		if event.kind == marketEventBook && worker.bookQueued != nil {
			worker.bookQueued.Delete(event.symbol.Symbol)
		}

		worked = true

		if worker.measurements != nil {
			worker.measurements.dispatchedAt = event.dispatchedAt
		}

		result := worker.measure(event)
		result.event.measuredAt = time.Now()

		if err := worker.outbox.Push(worker.ctx, result); err != nil {
			return worked, true
		}
	}

	return worked, false
}

func (worker *streamWorker) measure(event marketEvent) measurementResult {
	receivers := worker.receivers

	if worker.local {
		receivers = localReceivers(event.kind)
	}

	switch event.kind {
	case marketEventTicker:
		event.symbol.AppendTickerTo(event.ticker, receivers)
	case marketEventTrade:
		event.symbol.AppendTradeTo(event.trade, receivers)
	case marketEventLevel3:
		event.symbol.AppendLevel3(event.level3)
	}

	rows, err := worker.measurements.MeasureSymbol(
		event.symbol,
		event.tick,
		receivers,
	)

	return measurementResult{event: event, measurements: rows, err: err}
}

var localReceiverCache = map[marketEventKind][]types.SourceType{
	marketEventTicker: {types.SourceCVD, types.SourcePumpDump},
	marketEventTrade:  types.TradeReceivers,
	marketEventBook:   types.BookReceivers,
	marketEventLevel3: types.Level3Receivers,
}

func localReceivers(kind marketEventKind) []types.SourceType {
	return localReceiverCache[kind]
}

type eventResults struct {
	event        marketEvent
	received     int
	measurements []*types.Measurement
}

func (pipeline *streamPipeline) collect(wait *sync.WaitGroup) {
	defer wait.Done()
	pending := make(map[uint64]*eventResults)
	next := uint64(1)
	pipeline.nextSequence.Store(next)

	for {
		worked, err := pipeline.drainResults(pending)

		if err != nil {
			pipeline.fail(err)
			pipeline.cancel()
			return
		}

		for {
			results := pending[next]

			if results == nil {
				if _, dropped := pipeline.droppedSequences.LoadAndDelete(next); dropped {
					next++
					pipeline.nextSequence.Store(next)
					worked = true
					continue
				}

				break
			}

			if results.received != results.event.parts {
				break
			}

			delete(pending, next)
			pipeline.pendingCount.Add(-1)

			if !pipeline.commitInbox.TryPush(results) {
				pipeline.commitDropped.Add(1)
				results.event = marketEvent{}
				results.received = 0
				results.measurements = results.measurements[:0]
				eventResultsPool.Put(results)
				next++
				pipeline.nextSequence.Store(next)
				worked = true
				continue
			}

			next++
			pipeline.nextSequence.Store(next)
			worked = true
		}

		if worked {
			continue
		}

		select {
		case <-pipeline.ctx.Done():
			return
		case <-pipeline.resultWake:
		}
	}
}

func (pipeline *streamPipeline) drainResults(
	pending map[uint64]*eventResults,
) (bool, error) {
	worked := false

	for _, worker := range pipeline.workers {
		for range pipeline.config.drainLimit {
			result, ok := worker.outbox.Pop()

			if !ok {
				break
			}

			worked = true

			if result.err != nil {
				return worked, result.err
			}

			pipeline.stampCollected(&result.event)

			if result.event.kind != marketEventTicker {
				pipeline.forwardReady(result)
				continue
			}

			key := result.event.tickerSequence
			results := pending[key]

			if results == nil {
				results = eventResultsPool.Get().(*eventResults)
				results.event = result.event
				results.received = 0
				results.measurements = results.measurements[:0]
				pending[key] = results
				pipeline.pendingCount.Add(1)
			}

			results.received++
			results.measurements = append(results.measurements, result.measurements...)
		}
	}

	return worked, nil
}

func (pipeline *streamPipeline) commit(wait *sync.WaitGroup) {
	defer wait.Done()

	for {
		batch := pipeline.takeCommitBatch()

		if len(batch) == 0 {
			select {
			case <-pipeline.ctx.Done():
				return
			case <-pipeline.commitWake:
			}

			continue
		}

		pipeline.coalesceCommitBatch(batch)

		for _, results := range batch {
			if err := pipeline.commitEvent(results); err != nil {
				pipeline.fail(err)
				pipeline.cancel()
				return
			}
		}
	}
}

func (pipeline *streamPipeline) takeCommitBatch() []*eventResults {
	batch := make([]*eventResults, 0, pipeline.config.drainLimit)

	for range pipeline.config.drainLimit {
		results, ok := pipeline.commitInbox.Pop()

		if !ok {
			break
		}

		batch = append(batch, results)
	}

	return batch
}

func (pipeline *streamPipeline) coalesceCommitBatch(batch []*eventResults) {
	latest := make(map[string]int, len(batch))

	for index, results := range batch {
		if results == nil || results.event.kind != marketEventTicker ||
			results.event.symbol == nil {
			continue
		}

		latest[results.event.symbol.Symbol] = index
	}

	for index, results := range batch {
		if results == nil || results.event.kind != marketEventTicker ||
			results.event.symbol == nil {
			continue
		}

		results.event.skipAnalysis = latest[results.event.symbol.Symbol] != index
	}
}

func (pipeline *streamPipeline) commitEvent(results *eventResults) error {
	started := time.Now()
	defer pipeline.clocks.observe("commit", time.Since(started))
	event := results.event

	if !event.collectedAt.IsZero() {
		pipeline.clocks.observeHop("collect", "commit", started.Sub(event.collectedAt))
	}

	slices.SortStableFunc(
		results.measurements,
		func(left, right *types.Measurement) int {
			if sourceOrder := cmp.Compare(left.Source, right.Source); sourceOrder != 0 {
				return sourceOrder
			}

			if symbolOrder := cmp.Compare(left.Symbol, right.Symbol); symbolOrder != 0 {
				return symbolOrder
			}

			return cmp.Compare(left.Peer, right.Peer)
		},
	)

	if event.tick > pipeline.thesis.Tick {
		pipeline.thesis.Tick = event.tick
	}

	if event.at.After(pipeline.thesis.At) {
		pipeline.thesis.At = event.at
	}

	if len(results.measurements) > 0 {
		pipeline.measurementsDirty[event.symbol.Symbol] = true

		if pipeline.publisher.commit(
			pipeline.thesis,
			results.measurements,
		) {
			pipeline.resonanceDirty[event.symbol.Symbol] = true
		}
	}

	if event.kind == marketEventTicker {
		if event.ticker.Ask == nil || event.ticker.Bid == nil ||
			event.ticker.Ask.Sign() <= 0 || event.ticker.Bid.Sign() <= 0 {
			return fmt.Errorf(
				"stream: positive ticker quotes required for %s resonance mark",
				event.symbol.Symbol,
			)
		}

		mark := (event.ticker.Ask.Float64() + event.ticker.Bid.Float64()) / 2
		event.symbol.AppendResonanceMeasurement(&types.ResonanceMeasurement{
			Tick: event.tick,
			Mark: mark,
		})
		pipeline.resonanceDirty[event.symbol.Symbol] = true
	}

	if event.kind != marketEventTicker ||
		event.skipAnalysis ||
		pipeline.analyzed[event.symbol.Symbol] == event.tick ||
		(!pipeline.measurementsDirty[event.symbol.Symbol] &&
			!pipeline.resonanceDirty[event.symbol.Symbol]) {
		pipeline.markCommitted(event)
		return nil
	}

	logicStarted := time.Now()
	pipeline.clocks.observeHop("commit", "category", logicStarted.Sub(started))

	if err := pipeline.analyzer.Process(
		pipeline.thesis,
		event.symbol.Symbol,
		event.at,
		pipeline.measurementsDirty[event.symbol.Symbol],
		pipeline.resonanceDirty[event.symbol.Symbol],
	); err != nil {
		return fmt.Errorf("stream: analyzer process failed: %w", err)
	}

	delete(pipeline.resonanceDirty, event.symbol.Symbol)
	delete(pipeline.measurementsDirty, event.symbol.Symbol)
	pipeline.analyzed[event.symbol.Symbol] = event.tick

	symbolThesis, err := pipeline.thesis.ForSymbol(event.symbol.Symbol)

	if err != nil {
		return fmt.Errorf("stream: planner symbol scope failed: %w", err)
	}

	symbolThesis.At = event.at
	strategyStarted := time.Now()
	pipeline.clocks.observeHop("graph", "planner", strategyStarted.Sub(logicStarted))

	if err := pipeline.planner.Update(symbolThesis); err != nil {
		return fmt.Errorf("stream: planner update failed: %w", err)
	}

	pipeline.markCommitted(event)

	results.event = marketEvent{}
	results.received = 0
	results.measurements = results.measurements[:0]
	eventResultsPool.Put(results)

	return nil
}

func (pipeline *streamPipeline) forwardReady(result measurementResult) {
	results := eventResultsPool.Get().(*eventResults)
	results.event = result.event
	results.received = 1
	results.measurements = results.measurements[:0]
	results.measurements = append(results.measurements, result.measurements...)

	if pipeline.commitInbox.TryPush(results) {
		return
	}

	pipeline.commitDropped.Add(1)
	results.event = marketEvent{}
	results.received = 0
	results.measurements = results.measurements[:0]
	eventResultsPool.Put(results)
	pipeline.markCommitted(result.event)
}

func (pipeline *streamPipeline) markCommitted(event marketEvent) {
	pipeline.committedSequence.Add(1)
	pipeline.lastCommitNanos.Store(time.Now().UnixNano())

	select {
	case pipeline.progress <- struct{}{}:
	default:
	}
}
