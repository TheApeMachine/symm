package market

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	storyMeasurementsSubscriberID = "market:story"
	storyUIInterval               = 100 * time.Millisecond
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

	fh, err := os.Create(recordPath)

	if err != nil {
		errnie.Error(err)
	} else {
		story.recordFile = fh
		story.recorder = bufio.NewWriter(fh)
	}

	story.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	story.subscribers["measurements"] = story.broadcasts["measurements"].Subscribe(
		storyMeasurementsSubscriberID, 1024,
	)

	activate.Boot("market/story ready")

	return story
}

/*
Tick joins the latest measurements from the perspective signals and publishes them to the story.

UI events are rate-limited to storyUIInterval. Measurements flood the channel at high frequency
and selecting one "publish" case per measurement would starve the timer and flood the WebSocket.
Instead, we accumulate the latest value per source/symbol in local maps and flush them to the
"ui" broadcast on the timer tick — a single batch per interval regardless of measurement rate.
*/
func (story *Story) Tick() error {
	if story.recorder != nil {
		defer story.recorder.Flush()
	}

	incoming := story.subscribers["measurements"].Incoming
	uiFlush := time.NewTicker(storyUIInterval)

	defer uiFlush.Stop()

	latestConf := make(map[string]float64)

	for {
		select {
		case <-story.ctx.Done():
			return story.ctx.Err()

		case <-uiFlush.C:
			for source, conf := range latestConf {
				if !perspectives.DashboardGaugeSource(source) {
					continue
				}

				story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
					"source":     source,
					"confidence": conf,
				}})
			}

			now := time.Now()
			nowStr := now.UTC().Format(time.RFC3339Nano)

			story.publishEnginePulse(latestConf, nowStr)

		case row, ok := <-incoming:
			if !ok {
				return io.EOF
			}

			errnie.Debug("story.Tick", "measurement", row)

			if row == nil {
				errnie.Warn("nil measurement")
				continue
			}

			measurement, ok := row.Value.(perspectives.Measurement)

			if !ok {
				errnie.Warn("invalid measurement")
				continue
			}

			story.ingestMeasurement(measurement, latestConf)
		}
	}
}

func (story *Story) ingestMeasurement(
	measurement perspectives.Measurement,
	latestConf map[string]float64,
) {
	activate.Once("market/story:measurement source=" + measurement.Source.String())

	if story.recorder != nil {
		recorded := measurement

		if recorded.At.IsZero() {
			recorded.At = time.Now().UTC()
		}

		raw := errnie.Does(func() ([]byte, error) {
			return sonic.Marshal(recorded)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		activate.Once("market/story:recording")

		if _, writeErr := story.recorder.Write(append(raw, '\n')); writeErr != nil {
			errnie.Error(writeErr)
		}
	}

	story.ringWindow = AppendRingMeasurement(story.ringWindow, measurement)

	source := measurement.Source.String()

	if source != "" && perspectives.DashboardGaugeSource(source) {
		if measurement.SNR > latestConf[source] {
			latestConf[source] = measurement.SNR
		}
	}

	if measurement.Symbol == "" || measurement.Last <= 0 {
		return
	}

	if story.tree == nil {
		tree, err := perspectives.NewTree(story.ctx, []perspectives.Measurement{})

		if err != nil {
			errnie.Error(err)
			return
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

	if actionType == nil || *actionType == perspectives.ActionNone {
		return
	}

	if perspectives.IsEntryAction(*actionType) && !trading.DeskReady() {
		return
	}

	action := perspectives.ActionFromMeasurement(*actionType, measurement)

	if perspectives.IsEntryAction(*actionType) && action.Quantity <= 0 {
		return
	}

	activate.Once("market/story:action")

	story.raw.Send(&qpool.QValue[any]{
		Value: action,
	})
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
func (story *Story) publishEnginePulse(latestConf map[string]float64, ts string) {
	seq := story.pulseSeq.Add(1)

	if len(latestConf) == 0 {
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
	for _, value := range latestConf {
		sum += value
	}
	avg := sum / float64(len(latestConf))

	var variance float64
	for _, value := range latestConf {
		delta := value - avg
		variance += delta * delta
	}
	stddev := math.Sqrt(variance / float64(len(latestConf)))

	const gaugeFullSigma = 4.0

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":                   "engine_pulse",
		"ts":                      ts,
		"seq":                     seq,
		"phase":                   "ticking",
		"measurements":            len(latestConf),
		"open":                    0,
		"avg_prediction":          avg,
		"avg_prediction_multiple": avg / gaugeFullSigma,
		"avg_error":               stddev,
		"avg_error_multiple":      stddev / gaugeFullSigma,
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

	if story.recorder != nil {
		errnie.Error(story.recorder.Flush())
		story.recorder = nil
	}

	if story.recordFile != nil {
		errnie.Error(story.recordFile.Close())
		story.recordFile = nil
	}

	return nil
}
