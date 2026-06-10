package market

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	pool          *qpool.Q[any]
	bus           *internal.Bus
	measurements  *sync.Map
	tree          *logic.Tree
	crossSection  *CrossSection
	regime        *RegimeClassifier
	holdings      *logic.Holdings
	lastUIPublish time.Time
	audit         *audit.Writer
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
	holdings *logic.Holdings,
	auditWriter *audit.Writer,
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree()
	crossSection, crossSectionErr := LoadCrossSection(&CrossSectionOnce{})

	if crossSectionErr != nil {
		err = errors.Join(err, crossSectionErr)
	}

	regime, regimeErr := NewRegimeClassifier(crossSection)

	if regimeErr != nil {
		err = errors.Join(err, regimeErr)
	}

	return &Story{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelMeasurements, "story"),
			},
		),
		measurements: &sync.Map{},
		tree:         tree,
		crossSection: crossSection,
		regime:       regime,
		holdings:     holdings,
		audit:        auditWriter,
		err:          err,
	}
}

/*
Tick joins measurements from perspective signals, evaluates the playbook, and
streams regime and decision-tree frames to the UI as measurements arrive.
*/
func (story *Story) Tick() error {
	if errnie.Error(story.err) != nil {
		return story.err
	}

	for {
		if errnie.Error(story.ctx.Err()) != nil {
			return story.ctx.Err()
		}

		row, err := story.bus.Poll("measurements")

		if errnie.Error(err) != nil || row == nil {
			row, err = story.bus.Receive("measurements")
		}

		if errnie.Error(err) != nil {
			if errnie.Error(story.ctx.Err()) != nil {
				return story.ctx.Err()
			}

			continue
		}

		if row == nil {
			continue
		}

		measurement, ok := row.Value.(logic.Measurement)

		if !ok {
			errnie.Error(errors.New("story: invalid measurement"))
			continue
		}

		if story.regime != nil {
			story.regime.Observe(measurement)
		}

		if stats := story.tree.Stats(); stats != nil {
			stats.ObserveStoryTick()
		}

		raw, _ := story.measurements.LoadOrStore(measurement.Symbol, NewSymbolState())

		measurements := raw.(*SymbolState).Observe(measurement)

		var evaluation *logic.Evaluation

		if len(measurements) > 0 {
			trace := &logic.EvalTrace{}
			evaluation = story.tree.EvaluateTraced(measurements, story.holdings, trace)

			if evaluation == nil || evaluation.Action == nil {
				story.recordPlaybookEval(measurement.Symbol, trace, measurements)
			} else {
				story.recordPlaybookAction(measurement.Symbol, evaluation)
			}
		}

		if evaluation == nil || evaluation.Action == nil {
			story.publishUIFrames()
			continue
		}

		action := evaluation.Action

		if action.Symbol == "" {
			stamped := *action
			stamped.Symbol = measurement.Symbol
			action = &stamped
			evaluation.Action = action
		}

		errnie.Error(story.bus.Send(internal.ChannelRaw, "actions", action))

		story.publishUIFrames()
	}
}

func (story *Story) recordPlaybookEval(
	symbol string,
	trace *logic.EvalTrace,
	measurements []logic.Measurement,
) {
	if story.audit == nil || trace == nil || trace.Depth() < 2 {
		return
	}

	bottleneck := trace.Bottleneck()

	if bottleneck == nil || bottleneck.Key == "" {
		return
	}

	frame := map[string]any{
		"event":             "playbook_eval",
		"ts":                time.Now().UTC().Format(time.RFC3339Nano),
		"symbol":            symbol,
		"bottleneck_key":    bottleneck.Key,
		"bottleneck_label":  bottleneck.Label,
		"failed_conditions": trace.FailedConditionLabels(),
		"signals":           logic.SnapshotSignals(measurements),
	}

	dedupeKey := fmt.Sprintf("playbook_eval:%s:%s", symbol, bottleneck.Key)

	if !story.audit.TryEnqueueDeduped(dedupeKey, frame) {
		errnie.Error(errors.New("story: audit queue full"))
	}
}

func (story *Story) recordPlaybookAction(symbol string, evaluation *logic.Evaluation) {
	if story.audit == nil || evaluation == nil || evaluation.Action == nil {
		return
	}

	action := evaluation.Action

	frame := map[string]any{
		"event":  "playbook_action",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"symbol": symbol,
		"key":    evaluation.Key,
		"action": action.Type.String(),
		"side":   string(action.Side),
	}

	errnie.Error(story.audit.Enqueue(frame))
}

func (story *Story) publishUIFrames() {
	if !story.shouldPublishUI(time.Now()) {
		return
	}

	story.publishMarketRegime()
	story.publishDecisionTree()
}

func (story *Story) publishDecisionTree() {
	stats := story.tree.Stats()

	if stats == nil {
		return
	}

	errnie.Error(story.bus.Send(internal.ChannelUI, "decision_tree", stats.DecisionTreeFrame()))
}

func (story *Story) shouldPublishUI(now time.Time) bool {
	interval := viper.GetDuration("market.story.ui_interval")

	if interval <= 0 {
		interval = time.Second
	}

	if !story.lastUIPublish.IsZero() && now.Sub(story.lastUIPublish) < interval {
		return false
	}

	story.lastUIPublish = now

	return true
}

func (story *Story) publishMarketRegime() {
	if story.regime == nil {
		return
	}

	errnie.Error(story.regime.PublishFrame(story.bus))
}

/*
Regime exposes the cross-section regime classifier for desk sizing.
*/
func (story *Story) Regime() *RegimeClassifier {
	return story.regime
}

/*
TreeStats exposes playbook instrumentation for dashboards and decision recording.
*/
func (story *Story) TreeStats() *logic.TreeStats {
	if story.tree == nil {
		return nil
	}

	return story.tree.Stats()
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return story.bus.Close()
}
