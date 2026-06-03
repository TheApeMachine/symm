package market

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
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

	perspectives.BootstrapTelemetryManifest()

	activate.Boot("market/story ready")

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

			for source := range latest.bySource {
				clarity, count := latest.meanClarity(source)

				if count == 0 {
					continue
				}

				frame := map[string]any{
					"source":     source,
					"confidence": clarity,
					"count":      count,
				}

				if snr := latest.meanSNR(source); snr > 0 {
					frame["snr"] = snr
				}

				story.ui.Send(&qpool.QValue[any]{Value: frame})
			}

			now := time.Now()
			nowStr := now.UTC().Format(time.RFC3339Nano)

			story.publishEnginePulse(latest.sourceSNRMeans(), nowStr)
			latest = newGaugeReadings()
		}
	}
}

func (story *Story) ingestMeasurement(
	measurement perspectives.Measurement,
	latest gaugeReadings,
) error {
	activate.Once("market/story:measurement source=" + measurement.Source.String())

	if story.recorder != nil {
		recorded := measurement

		if recorded.At.IsZero() {
			recorded.At = time.Now().UTC()
		}

		raw, err := sonic.Marshal(recorded)

		if err != nil {
			return fmt.Errorf("marshal measurement: %w", err)
		}

		activate.Once("market/story:recording")

		if _, writeErr := story.recorder.Write(append(raw, '\n')); writeErr != nil {
			return fmt.Errorf("write measurement record: %w", writeErr)
		}
	}

	story.ringWindow = AppendRingMeasurement(story.ringWindow, measurement)

	perspectives.DefaultTelemetryRegistry().ObserveMeasurement(measurement)

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
		activate.Once("market/story:playbook-tree")
	}

	snapshots := RingSnapshot(story.ringWindow, measurement.Symbol)
	story.tree.ResetWalk()

	actionType := story.tree.WalkContext(
		perspectives.BranchContext{
			Measurements: snapshots,
			Observations: story.observations(measurement.Symbol),
			Metrics: map[string]float64{
				"last": measurement.Last,
			},
		},
		story.tree.Branches()...,
	)

	blockReason := ""

	if actionType == nil || *actionType == perspectives.ActionNone {
		if err := story.maybeWritePlaybookWalkAudit(measurement, blockReason); err != nil {
			return fmt.Errorf("playbook walk audit: %w", err)
		}

		return nil
	}

	if perspectives.IsEntryAction(*actionType) {
		blockReason = "desk_not_ready"

		if err := story.maybeWritePlaybookWalkAudit(measurement, blockReason); err != nil {
			return fmt.Errorf("playbook walk audit: %w", err)
		}

		return nil
	}

	action := perspectives.ActionFromMeasurement(*actionType, measurement)

	if perspectives.IsEntryAction(*actionType) && action.Quantity <= 0 {
		blockReason = "entry_quantity_zero"

		if err := story.maybeWritePlaybookWalkAudit(measurement, blockReason); err != nil {
			return fmt.Errorf("playbook walk audit: %w", err)
		}

		return nil
	}

	if err := story.maybeWritePlaybookWalkAudit(measurement, blockReason); err != nil {
		return fmt.Errorf("playbook walk audit: %w", err)
	}

	activate.Once("market/story:action")

	story.raw.Send(&qpool.QValue[any]{
		Value: action,
	})

	return nil
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

// publishEnginePulse emits a system-health summary derived from the latest
// per-source confidence map. The prediction chart plots avg_prediction_multiple.
func (story *Story) publishEnginePulse(latestSNR map[string]float64, ts string) {
	seq := story.pulseSeq.Add(1)

	if len(latestSNR) == 0 {
		story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":        "engine_pulse",
			"ts":           ts,
			"seq":          seq,
			"phase":        "ticking",
			"measurements": 0,
			"open":         0,
		}})
		return
	}

	var sum float64
	for _, value := range latestSNR {
		sum += value
	}
	avg := sum / float64(len(latestSNR))

	var variance float64
	for _, value := range latestSNR {
		delta := value - avg
		variance += delta * delta
	}
	stddev := math.Sqrt(variance / float64(len(latestSNR)))

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":                   "engine_pulse",
		"ts":                      ts,
		"seq":                     seq,
		"phase":                   "ticking",
		"measurements":            len(latestSNR),
		"open":                    0,
		"avg_prediction":          avg,
		"avg_prediction_multiple": avg / perspectives.GaugeFullSigma,
		"avg_error":               stddev,
		"avg_error_multiple":      stddev / perspectives.GaugeFullSigma,
	}})
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
