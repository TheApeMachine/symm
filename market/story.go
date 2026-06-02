package market

import (
	"bufio"
	"container/ring"
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
	buffer      *ring.Ring
	trees       []*perspectives.Tree
	lastGauge   map[string]time.Time
	recordFile  *os.File
	recorder    *bufio.Writer
	pulseSeq    atomic.Int64
}

func NewStory(ctx context.Context, pool *qpool.Q) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		buffer:      ring.New(128),
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		trees:       make([]*perspectives.Tree, 0),
		lastGauge:   make(map[string]time.Time),
	}

	story.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	recordPath := viper.GetViper().GetString("trading.record.file")

	fh, err := os.Create(recordPath)

	if err != nil {
		errnie.Error(err)
	} else {
		story.recordFile = fh
		story.recorder = bufio.NewWriter(fh)
	}

	for _, channel := range []string{"measurements", "actions"} {
		story.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	story.subscribers["measurements"] = story.broadcasts["measurements"].Subscribe(
		storyMeasurementsSubscriberID, 128,
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

	// Latest values accumulated between timer ticks. Written by the measurement
	// case, read and published by the timer case. Both run on the same goroutine
	// so no mutex needed.
	latestConf := make(map[string]float64) // source name → confidence
	latestMark := make(map[string]float64) // symbol → last price

	for {
		select {
		case <-story.ctx.Done():
			return story.ctx.Err()

		case <-uiFlush.C:
			// Publish one confidence row per source — at most one WebSocket
			// frame per source per interval, regardless of measurement rate.
			for source, conf := range latestConf {
				story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
					"source":     source,
					"confidence": conf,
				}})
			}

			now := time.Now()
			nowStr := now.UTC().Format(time.RFC3339Nano)

			if len(latestMark) > 0 {
				for symbol, price := range latestMark {
					story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
						"event":  "mark",
						"ts":     nowStr,
						"symbol": symbol,
						"price":  price,
					}})
				}
			}

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

			story.buffer.Value = measurement
			story.buffer.Next()

			// Accumulate — do NOT publish here. The timer case above handles it.
			// SNR is in noise-sigma units; the gauge maps 0..4σ onto 0..100%.
			if source := measurement.Source.String(); source != "" {
				latestConf[source] = measurement.SNR
			}

			if measurement.Last > 0 && measurement.Symbol != "" {
				latestMark[measurement.Symbol] = measurement.Last
			}

			actions := make([]*perspectives.ActionType, 0)

			for _, tree := range story.trees {
				if buffered, bufferedOK := story.buffer.Value.(perspectives.Measurement); bufferedOK {
					tree.AddMeasurement(buffered)
				}

				if action := tree.Action(); action != nil {
					actions = append(actions, action)
				}
			}

			if len(story.trees) == 0 || len(actions) == 0 {
				measurements := make([]perspectives.Measurement, 0)

				story.buffer.Do(func(value any) {
					if value == nil {
						return
					}

					buffered, bufferedOK := value.(perspectives.Measurement)

					if !bufferedOK {
						return
					}

					measurements = append(measurements, buffered)
				})

				story.trees = append(story.trees, errnie.Does(func() (*perspectives.Tree, error) {
					return perspectives.NewTree(story.ctx, measurements)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value())

				activate.Once("market/story:playbook-tree")
			}

			for _, action := range actions {
				activate.Once("market/story:action")
				story.broadcasts["actions"].Send(&qpool.QValue[any]{Value: action})
			}
		}
	}
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
	for _, v := range latestConf {
		sum += v
	}
	avg := sum / float64(len(latestConf))

	var variance float64
	for _, v := range latestConf {
		d := v - avg
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(len(latestConf)))

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":          "engine_pulse",
		"ts":             ts,
		"seq":            seq,
		"phase":          "ticking",
		"measurements":   len(latestConf),
		"open":           0,
		"avg_prediction": avg,
		"avg_error":      stddev,
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
