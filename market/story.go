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
	"github.com/theapemachine/symm/audit"
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
	ui          *qpool.BroadcastGroup
	raw         *qpool.BroadcastGroup
	streams     *focus.Set
	tree        *perspectives.Tree
	ringWindow  []perspectives.Measurement
	lastGauge   map[string]time.Time
	recordFile  *os.File
	recorder    *bufio.Writer
	auditWriter *audit.Writer
	auditDedup  playbookWalkDedup
	pulseSeq    atomic.Int64
}

func NewStory(ctx context.Context, pool *qpool.Q, streams *focus.Set) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		streams:     streams,
		ringWindow:  make([]perspectives.Measurement, 0, StoryRingCapacity),
		lastGauge:   make(map[string]time.Time),
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

	auditWriter, err := audit.OpenWriter()

	if err != nil {
		cancel()
		errnie.Error(fmt.Errorf("story: audit writer: %w", err), "story")
		return nil
	}

	story.auditWriter = auditWriter
	story.auditDedup = newPlaybookWalkDedup()

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
	errnie.Info("market/story:measurement source=" + measurement.Source.String())

	if story.recorder != nil {
		recorded := measurement

		if recorded.At.IsZero() {
			recorded.At = time.Now().UTC()
		}

		raw, err := sonic.Marshal(recorded)

		if err != nil {
			return fmt.Errorf("marshal measurement: %w", err)
		}

		errnie.Info("market/story:recording")

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

	if story.tree == nil {
		tree, err := story.loadPlaybookTree()

		if err != nil {
			return fmt.Errorf("playbook tree: %w", err)
		}

		story.tree = tree
		errnie.Info("market/story:playbook-tree")
	}

	snapshots := RingSnapshot(story.ringWindow, measurement.Symbol)
	regime := perspectives.ClassifyRegime(snapshots)
	story.publishRegime(measurement.Symbol, regime)
	story.tree.ResetWalk()

	actionType := story.tree.WalkContext(
		perspectives.BranchContext{
			Measurements: snapshots,
			Observations: story.observations(measurement.Symbol),
			Regime:       regime.Regime,
			Metrics: map[string]float64{
				"last": measurement.Last,
			},
		},
		story.tree.Branches()...,
	)

	action := perspectives.ActionFromMeasurement(actionType, measurement)

	story.raw.Send(&qpool.QValue[any]{Value: action})

	if actionType != perspectives.ActionNone {
		story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "action",
			"type":   action.Type.String(),
			"symbol": action.Symbol,
		}})
	}

	return nil
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

func (story *Story) observations(symbol string) map[perspectives.ObservationType]float64 {
	observations := make(map[perspectives.ObservationType]float64, 1)

	if story.streams != nil && story.streams.Has(symbol) {
		observations[perspectives.ObservationHolding] = 1

		return observations
	}

	observations[perspectives.ObservationNotHolding] = 1

	return observations
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

	if story.auditWriter != nil {
		if auditErr := story.auditWriter.Close(); auditErr != nil && closeErr == nil {
			closeErr = auditErr
		}

		story.auditWriter = nil
	}

	return closeErr
}

func (story *Story) loadPlaybookTree() (*perspectives.Tree, error) {
	if viper.GetBool("market.perspectives.fixture_playbook") {
		return perspectives.NewTreeFromBranches(story.ctx, perspectives.FixturePlaybookBranches())
	}

	return perspectives.NewTree(story.ctx, []perspectives.Measurement{})
}
