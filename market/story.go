package market

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	storyMeasurementsSubscriberID = "market:story"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	ui           *qpool.BroadcastGroup
	raw          *qpool.BroadcastGroup
	streams      *focus.Set
	thoughts     []perspectives.Thought
	playbookLoaded bool
	reasonStates map[string]*perspectives.ReasonState
	positions    map[string]*perspectives.PositionState
	ringWindow   []perspectives.Measurement
	lastGauge    map[string]time.Time
	recordFile *os.File
	recorder   *bufio.Writer
	pulseSeq   atomic.Int64
}

func NewStory(ctx context.Context, pool *qpool.Q, streams *focus.Set) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		streams:      streams,
		reasonStates: make(map[string]*perspectives.ReasonState),
		positions:    make(map[string]*perspectives.PositionState),
		ringWindow:   make([]perspectives.Measurement, 0, StoryRingCapacity),
		lastGauge:    make(map[string]time.Time),
	}

	story.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	story.raw = pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

	recordPath := viper.GetViper().GetString("trading.record.file")

	if recordPath != "" {
		fh, err := os.Create(recordPath)

		if err != nil {
			cancel()
			errnie.Error(fmt.Errorf("story: record file %q: %w", recordPath, err), "story")
			return nil
		}

		story.recordFile = fh
		story.recorder = bufio.NewWriter(fh)
	}

	story.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	story.subscribers["measurements"] = story.broadcasts["measurements"].Subscribe(
		storyMeasurementsSubscriberID, 1024,
	)

	errnie.Info("market/story ready", "market/story")

	return story
}

/*
Tick joins the latest measurements from the perspective signals and publishes them to the story.

UI events are rate-limited to storyUIInterval. Measurements flood the channel at high frequency
and selecting one "publish" case per measurement would starve the timer and flood the WebSocket.
Instead, we accumulate per-source/symbol readings between UI ticks and flush
cross-sectional means on the timer — then reset the window so each gauge frame
reflects the last interval, not the lifetime of the process.
*/
func (story *Story) Tick() error {
	if story.recorder != nil {
		defer story.recorder.Flush()
	}

	incoming := story.subscribers["measurements"].Incoming
	latest := newGaugeReadings()

	for {
		select {
		case <-story.ctx.Done():
			return story.ctx.Err()
		case row, ok := <-incoming:
			if !ok {
				continue
			}

			if row == nil {
				errnie.Error(nil, "story: nil measurement envelope")
				continue
			}

			measurement, typeOK := row.Value.(perspectives.Measurement)

			if !typeOK {
				errnie.Error(nil, "story: invalid measurement type %T", row.Value)
				continue
			}

			if err := story.ingestMeasurement(measurement, latest); err != nil {
				errnie.Error(err, "story: ingest %s", measurement.Symbol)
				continue
			}

			latest = newGaugeReadings()
		}
	}
}

func (story *Story) ingestMeasurement(
	measurement perspectives.Measurement,
	latest gaugeReadings,
) error {
	if story.recorder != nil {
		recorded := measurement

		if recorded.At.IsZero() {
			recorded.At = time.Now().UTC()
		}

		raw, err := sonic.Marshal(recorded)

		if err != nil {
			return fmt.Errorf("marshal measurement: %w", err)
		}

		if _, writeErr := story.recorder.Write(append(raw, '\n')); writeErr != nil {
			return fmt.Errorf("write measurement record: %w", writeErr)
		}
	}

	story.ringWindow = AppendRingMeasurement(story.ringWindow, measurement)

	source := measurement.Source.String()

	if source != "" {
		latest.record(source, measurement.Symbol, measurement.Confidence, measurement.SNR)
	}

	if measurement.Symbol == "" || measurement.Last <= 0 {
		return nil
	}

	if !story.playbookLoaded {
		thoughts, err := story.loadThoughts()

		if err != nil {
			return fmt.Errorf("playbook: %w", err)
		}

		story.thoughts = thoughts
		story.playbookLoaded = true
	}

	snapshots := RingSnapshot(story.ringWindow, measurement.Symbol)
	regime := perspectives.ClassifyRegime(snapshots)
	story.publishRegime(measurement.Symbol, regime)

	if len(story.thoughts) == 0 {
		return nil
	}

	context := perspectives.NewWindowReason(
		snapshots, regime.Regime, story.positionState(measurement),
	)

	act, found := perspectives.EvaluateStateful(
		story.thoughts, context, story.reasonState(measurement.Symbol),
	)

	if !found || act.Type == perspectives.ActionNone {
		return nil
	}

	// The trader publishes the decision card (with the accept/reject reason) once
	// the action clears or fails its gates; the story only forwards the verdict.
	story.raw.Send(&qpool.QValue[any]{Value: perspectives.ActionFromAct(act, measurement)})

	return nil
}

// positionState projects what the story knows about the open position into the
// view the reasoning language reasons over. Holding is the fill-sourced focus set;
// entry, peak, and elapsed are derived from the prices observed since the position
// opened. (The exact fill price arrives with the execution events Stage 6 will wire
// from the paper/live socket.)
func (story *Story) positionState(measurement perspectives.Measurement) perspectives.PositionState {
	symbol := measurement.Symbol
	holding := story.streams != nil && story.streams.Has(symbol)

	now := measurement.At
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if !holding {
		delete(story.positions, symbol)

		return perspectives.PositionState{Holding: false, Last: measurement.Last, Now: now}
	}

	state, ok := story.positions[symbol]

	if !ok {
		state = &perspectives.PositionState{
			Holding: true, EntryPrice: measurement.Last, Peak: measurement.Last, EntryAt: now,
		}
		story.positions[symbol] = state
	}

	state.Holding = true
	state.Last = measurement.Last
	state.Now = now

	if measurement.Last > state.Peak {
		state.Peak = measurement.Last
	}

	return *state
}

// reasonState returns the symbol's cross-tick reasoning memory, created on first
// use — the same per-symbol latch the replay ledger threads, so live matches replay.
func (story *Story) reasonState(symbol string) *perspectives.ReasonState {
	state, ok := story.reasonStates[symbol]

	if !ok {
		state = perspectives.NewReasonState()
		story.reasonStates[symbol] = state
	}

	return state
}

/*
publishRegime sends the current price-action regime and its radar axes to the
dashboard. The same classification feeds BranchContext.Regime, so what the radar
shows is exactly what the decision tree branches on.
*/
func (story *Story) publishRegime(symbol string, regime perspectives.RegimeFeatures) {
	axes := regime.Radar()

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"chart":      "regime",
		"symbol":     symbol,
		"regime":     regime.Regime.String(),
		"volatility": axes["volatility"],
		"trend":      axes["trend"],
		"bullish":    axes["bullish"],
		"bearish":    axes["bearish"],
		"choppiness": axes["choppiness"],
	}})
}

// loadThoughts reads the reasoning playbook the optimizer writes. A missing file
// is not fatal: the story simply does not act until a playbook is tuned.
func (story *Story) loadThoughts() ([]perspectives.Thought, error) {
	path := playbookPath()

	raw, err := os.ReadFile(path)

	if err != nil {
		errnie.Info("no playbook at "+path+" — story idle until one is tuned", "market/story")

		return nil, nil
	}

	return perspectives.ParseThoughts(raw)
}

// playbookPath resolves where the reasoning playbook lives — the same file the
// optimizer (make tune) writes, so a fresh tune feeds the next run.
func playbookPath() string {
	if path := os.Getenv("SYMM_PERSPECTIVES_FILE"); path != "" {
		return path
	}

	if path := viper.GetString("market.perspectives.file"); path != "" {
		return path
	}

	return "market/perspectives/cfg/perspectives.yaml"
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()

	if subscriber := story.subscribers["measurements"]; subscriber != nil {
		if broadcast := story.broadcasts["measurements"]; broadcast != nil {
			broadcast.Unsubscribe(subscriber.ID)
		}
	}

	var closeErr error

	if story.recorder != nil {
		if flushErr := story.recorder.Flush(); flushErr != nil {
			closeErr = flushErr
		}

		story.recorder = nil
	}

	if story.recordFile != nil {
		if fileErr := story.recordFile.Close(); fileErr != nil && closeErr == nil {
			closeErr = fileErr
		}

		story.recordFile = nil
	}

	return closeErr
}

