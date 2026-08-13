package trader

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/theapemachine/symm/types"
)

type measurementResult struct {
	event        marketEvent
	measurements []*types.Measurement
	err          error
}

type streamWorker struct {
	ctx          context.Context
	drainLimit   int
	local        bool
	receivers    []types.SourceType
	measurements *Measurements
	inbox        *lane[marketEvent]
	outbox       *lane[measurementResult]
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

	return &streamWorker{
		ctx:          pipeline.ctx,
		drainLimit:   pipeline.config.drainLimit,
		local:        local,
		receivers:    receivers,
		measurements: measurements,
		inbox:        inbox,
		outbox:       outbox,
		wake:         wake,
	}, nil
}

func (pipeline *streamPipeline) Dispatch(event marketEvent) error {
	if event.symbol == nil || event.sequence == 0 {
		return fmt.Errorf("stream: complete event identity required")
	}

	localIndex := int(event.symbol.ID) & (len(pipeline.local) - 1)
	targets := []*streamWorker{pipeline.local[localIndex]}

	if event.kind == marketEventTicker {
		targets = append(targets, pipeline.cross...)
	}

	event.parts = len(targets)

	for _, worker := range targets {
		if err := worker.inbox.Push(pipeline.ctx, event); err != nil {
			return err
		}
	}

	return nil
}

func (worker *streamWorker) run(wait *sync.WaitGroup) {
	defer wait.Done()

	for {
		worked := false

		for range worker.drainLimit {
			event, ok := worker.inbox.Pop()

			if !ok {
				break
			}

			worked = true
			result := worker.measure(event)

			if err := worker.outbox.Push(worker.ctx, result); err != nil {
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

func localReceivers(kind marketEventKind) []types.SourceType {
	switch kind {
	case marketEventTicker:
		return []types.SourceType{types.SourceCVD, types.SourcePumpDump}
	case marketEventTrade:
		return types.TradeReceivers
	case marketEventBook:
		return types.BookReceivers
	case marketEventLevel3:
		return types.Level3Receivers
	default:
		return nil
	}
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

	for {
		worked, err := pipeline.drainResults(pending)

		if err != nil {
			pipeline.fail(err)
			pipeline.cancel()
			return
		}

		for {
			results := pending[next]

			if results == nil || results.received != results.event.parts {
				break
			}

			delete(pending, next)

			if err := pipeline.commitInbox.Push(pipeline.ctx, results); err != nil {
				return
			}

			next++
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

			results := pending[result.event.sequence]

			if results == nil {
				results = &eventResults{event: result.event}
				pending[result.event.sequence] = results
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
		worked := false

		for range pipeline.config.drainLimit {
			results, ok := pipeline.commitInbox.Pop()

			if !ok {
				break
			}

			worked = true

			if err := pipeline.commitEvent(results); err != nil {
				pipeline.fail(err)
				pipeline.cancel()
				return
			}
		}

		if worked {
			continue
		}

		select {
		case <-pipeline.ctx.Done():
			return
		case <-pipeline.commitWake:
		}
	}
}

func (pipeline *streamPipeline) commitEvent(results *eventResults) error {
	event := results.event
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
		pipeline.measurementsDirty = true

		if pipeline.publisher.commit(
			pipeline.thesis,
			results.measurements,
		) {
			pipeline.resonanceDirty = true
		}
	}

	if event.kind != marketEventTicker ||
		pipeline.analyzed[event.symbol.Symbol] == event.tick ||
		!pipeline.measurementsDirty {
		pipeline.markCommitted(event)
		return nil
	}

	if err := pipeline.analyzer.Process(pipeline.thesis, pipeline.resonanceDirty); err != nil {
		return fmt.Errorf("stream: analyzer process failed: %w", err)
	}

	pipeline.resonanceDirty = false
	pipeline.measurementsDirty = false
	pipeline.analyzed[event.symbol.Symbol] = event.tick

	if err := pipeline.planner.Update(pipeline.thesis); err != nil {
		return fmt.Errorf("stream: planner update failed: %w", err)
	}

	pipeline.markCommitted(event)

	return nil
}

func (pipeline *streamPipeline) markCommitted(event marketEvent) {
	pipeline.progressMu.Lock()
	pipeline.committedSequence = event.sequence
	pipeline.progressMu.Unlock()

	select {
	case pipeline.progress <- struct{}{}:
	default:
	}
}
