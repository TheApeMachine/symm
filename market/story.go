package market

import (
	"bufio"
	"container/ring"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	storyMeasurementsSubscriberID  = "market:story"
	defaultStoryDecisionMinSamples = 1
	// playbookRecheckInterval paces playbook file probes while none is loaded.
	playbookRecheckInterval = 5 * time.Second
	// decisionTreePublishInterval paces the full-tree dashboard frame. The tree
	// was previously rebuilt and broadcast per measurement — the story's largest
	// allocation source and a firehose at the hub.
	decisionTreePublishInterval = 250 * time.Millisecond
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
	err                     error
	pool                    *qpool.Q[any]
	broadcasts              map[string]*qpool.BroadcastGroup
	subscribers             map[string]*qpool.BroadcastConsumer
	ui                      *qpool.BroadcastGroup
	raw                     *qpool.BroadcastGroup
	regimeFeatures          map[string]perspectives.RegimeFeatures
	thoughts                []reasoning.Thought
	predictionCalibrator    *numeric.SignalCalibrator
	predictionSurpriseField *types.CategorySurpriseField
	forwardFeedback         *forwardFeedback
	playbookLoaded          bool
	reasonStates            map[string]*reasoning.ReasonState
	positions               map[string]*reasoning.PositionState
	symbolWindows           map[string]*ring.Ring
	windowCapacity          int
	windowReason            reasoning.WindowReason
	lastGauge               map[string]time.Time
	recordFile              *os.File
	recorder                *bufio.Writer
	audit                   *audit.Writer
	quoteVolumeBase         map[string]float64 // 24h ticker volume in base units; notional = base × last
	bookEnricher            func(types.Measurement) types.Measurement
	quoteReady              func(string) bool
	positionHeld            func(string) bool

	decisionTree      []reasoning.TreeNode
	nodeReached       map[string]int
	nodeHeld          map[string]int
	decisionEvals     int
	recentDecisions   []map[string]any
	reasonTrace       reasoning.ReasonTrace
	condHeld          map[string][]int
	playbookCheckedAt time.Time
	treePublishedAt   time.Time
}

func NewStory(ctx context.Context, pool *qpool.Q[any]) (*Story, error) {
	auditPool := audit.NewWriterPool()
	auditWriter, err := auditPool.OpenConfigured()

	if err != nil {
		return nil, fmt.Errorf("market/story: audit: %w", err)
	}

	story := NewStoryWithAudit(ctx, pool, auditWriter)

	if story == nil {
		return nil, fmt.Errorf("market/story: construction failed")
	}

	return story, nil
}

func NewStoryWithAudit(ctx context.Context, pool *qpool.Q[any], auditWriter *audit.Writer) *Story {
	ctx, cancel := context.WithCancel(ctx)

	predictionSurpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryStochasticNoise,
		types.CategoryStochasticBalance,
		types.CategorySynchronizedDrift,
		types.CategoryOrganicTrend,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "story")
		return nil
	}

	measurementBuffer, err := MeasurementBuffer()

	if err != nil {
		cancel()
		errnie.Error(err, "story")
		return nil
	}

	story := &Story{
		ctx:                     ctx,
		cancel:                  cancel,
		pool:                    pool,
		broadcasts:              make(map[string]*qpool.BroadcastGroup),
		subscribers:             make(map[string]*qpool.BroadcastConsumer),
		reasonStates:            make(map[string]*reasoning.ReasonState),
		positions:               make(map[string]*reasoning.PositionState),
		regimeFeatures:          make(map[string]perspectives.RegimeFeatures),
		predictionSurpriseField: predictionSurpriseField,
		forwardFeedback:         newForwardFeedbackFromConfig(),
		symbolWindows:           make(map[string]*ring.Ring),
		windowCapacity:          measurementBuffer,
		lastGauge:               make(map[string]time.Time),
		nodeReached:             make(map[string]int),
		nodeHeld:                make(map[string]int),
		condHeld:                make(map[string][]int),
		quoteVolumeBase:         make(map[string]float64),
	}

	story.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	story.raw = pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

	story.predictionCalibrator = numeric.NewSignalCalibrator(
		predictionDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"stochastic_noise", "stochastic_balance", "synchronized_drift", "organic_trend"},
		[]float64{0.30, 0.30, 0.25, 0.15},
		numeric.DefaultCalibratorConfig("strength"),
		"",
	)

	recordPath := viper.GetViper().GetString("trading.record.file")

	if recordPath != "" {
		// Preserve the previous capture instead of truncating it: a day of
		// recorded market history (yesterday's only profitable session, say)
		// must not vanish because someone restarted the engine.
		rotateExistingCapture(recordPath)

		fh, err := os.Create(recordPath)

		if err != nil {
			cancel()
			errnie.Error(fmt.Errorf("story: record file %q: %w", recordPath, err), "story")
			return nil
		}

		story.recordFile = fh
		story.recorder = bufio.NewWriter(fh)
	}

	story.audit = auditWriter

	story.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", viper.GetDuration("system.queue.ttl"),
	)

	story.subscribers["measurements"] = story.broadcasts["measurements"].Subscribe(
		storyMeasurementsSubscriberID, measurementBuffer,
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

	measurements := story.subscribers["measurements"]

	for {
		row, err := measurements.Wait(story.ctx)

		if err != nil {
			return err
		}

		if row == nil || row.Value == nil {
			errnie.Warn("story: nil measurement envelope")
			continue
		}

		if err := errnie.Error(
			story.ingestMeasurement(signalpool.GetMeasurement(row)),
		); err != nil {
			continue
		}
	}
}

func (story *Story) ingestMeasurement(
	measurement types.Measurement,
) error {
	if measurement.At.IsZero() {
		measurement.At = time.Now().UTC()
	}

	measurement = story.stampQuoteNotional(measurement)

	recorded := story.enrichMeasurementBook(measurement)
	story.recordMeasurement(recorded)

	prediction, telemetry, chartPoints, predicted, err := story.observePredictionFeedback(measurement)

	if err != nil {
		return errnie.Error(err)
	}

	if predicted {
		prediction = story.stampQuoteNotional(prediction)
		story.recordMeasurement(story.enrichMeasurementBook(prediction))
		story.rememberMeasurement(prediction)
		story.publishPredictionGauge(prediction, telemetry)
		story.publishPredictionChart(measurement.Symbol, chartPoints)
	}

	if len(story.thoughts) == 0 {
		// Re-check for a playbook on a cadence, not on every measurement: at
		// thousands of measurements per second the miss path was a continuous
		// stream of os.ReadFile syscalls.
		if time.Since(story.playbookCheckedAt) >= playbookRecheckInterval {
			story.playbookCheckedAt = time.Now()

			if story.thoughts, story.err = story.loadThoughts(); story.err != nil {
				return errnie.Error(story.err)
			}

			story.decisionTree = reasoning.BuildTree(story.thoughts)
			story.publishDecisionTree()
		}
	}

	story.rememberMeasurement(measurement)

	snapshots := story.ringSnapshot(measurement.Symbol)

	if len(snapshots) < storyDecisionMinSamples() {
		return nil
	}

	features := perspectives.ClassifyRegime(snapshots)
	story.regimeFeatures[measurement.Symbol] = features
	story.publishMarketRegime()

	if len(story.thoughts) == 0 {
		return nil
	}

	context := story.windowReason.Reset(
		snapshots,
		features.Regime,
		story.positionState(measurement),
	)

	story.reasonTrace.Nodes = story.reasonTrace.Nodes[:0]

	act, found := reasoning.EvaluateStatefulTraced(
		story.thoughts, context, story.reasonState(measurement.Symbol), &story.reasonTrace,
	)

	story.foldDecisionTrace(measurement.Symbol, act, found)

	if !found || act.Type == reasoning.ActionNone {
		return nil
	}

	if reasoning.IsShortAct(act) && !viper.GetBool("trading.margin_enabled") {
		return nil
	}

	action := reasoning.ActionFromAct(act, measurement)
	action.Regime = features.Regime

	if story.quoteReady != nil && !story.quoteReady(measurement.Symbol) {
		return nil
	}

	if err := story.writePlaybookAudit(
		measurement,
		story.regimeFeatures[measurement.Symbol],
		action,
		story.reasonTrace,
	); errnie.Error(err) != nil {
		return errnie.Error(err)
	}

	story.raw.Send(&qpool.QValue[any]{
		Type:  "action",
		Value: action,
	})

	if reasoning.IsEntryAction(act.Type) {
		story.reasonState(measurement.Symbol).Reset()
	}

	return nil
}

func storyDecisionMinSamples() int {
	configured := viper.GetInt("story.decision_min_samples")

	if configured > 0 {
		return configured
	}

	return defaultStoryDecisionMinSamples
}

func (story *Story) ringSnapshot(symbol string) []types.Measurement {
	window, ok := story.symbolWindows[symbol]

	if !ok {
		return nil
	}

	capacity := window.Len()
	snapshots := make([]types.Measurement, 0, capacity)

	window.Do(func(value any) {
		measurement, ok := value.(types.Measurement)

		if !ok {
			return
		}

		snapshots = append(snapshots, measurement)
	})

	return snapshots
}

/*
foldDecisionTrace accumulates the per-node reachable/held counts from one
evaluation and, on a cadence, publishes the live decision tree to the dashboard.
*/
func (story *Story) foldDecisionTrace(symbol string, act reasoning.Act, found bool) {
	for index := range story.reasonTrace.Nodes {
		node := story.reasonTrace.Nodes[index]

		if node.Reachable {
			story.nodeReached[node.Key]++

			if len(node.Leaves) > 0 {
				held := story.condHeld[node.Key]

				if len(held) != len(node.Leaves) {
					held = make([]int, len(node.Leaves))
					story.condHeld[node.Key] = held
				}

				for leafIndex, leafHolds := range node.Leaves {
					if leafHolds {
						held[leafIndex]++
					}
				}
			}
		}

		if node.Fires {
			story.nodeHeld[node.Key]++
		}
	}

	story.decisionEvals++

	if found && act.Type != reasoning.ActionNone {
		story.recentDecisions = append(story.recentDecisions, map[string]any{
			"symbol": symbol,
			"action": act.Type.String(),
			"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		})

		if len(story.recentDecisions) > 20 {
			trimmed := make([]map[string]any, 20)
			copy(trimmed, story.recentDecisions[len(story.recentDecisions)-20:])
			story.recentDecisions = trimmed
		}
	}

	story.publishDecisionTree()
}

/*
publishDecisionTree ships the playbook structure with each node's live
reached/held counts to the dashboard, so the decision tree shows where
evaluations travel and where they die.
*/
func (story *Story) publishDecisionTree() {
	if story.ui == nil || len(story.decisionTree) == 0 {
		return
	}

	if time.Since(story.treePublishedAt) < decisionTreePublishInterval {
		return
	}

	story.treePublishedAt = time.Now()

	nodes := make([]map[string]any, 0, len(story.decisionTree))

	for index := range story.decisionTree {
		node := story.decisionTree[index]

		conditions := make([]map[string]any, 0, len(node.Conditions))
		held := story.condHeld[node.Key]

		for conditionIndex := range node.Conditions {
			leafHeld := 0

			if conditionIndex < len(held) {
				leafHeld = held[conditionIndex]
			}

			conditions = append(conditions, map[string]any{
				"label":   node.Conditions[conditionIndex].Label,
				"negated": node.Conditions[conditionIndex].Negated,
				"held":    leafHeld,
			})
		}

		nodes = append(nodes, map[string]any{
			"key":        node.Key,
			"depth":      node.Depth,
			"parent":     node.Parent,
			"label":      node.Label,
			"action":     node.Action,
			"combinator": node.Combinator,
			"reached":    story.nodeReached[node.Key],
			"held":       story.nodeHeld[node.Key],
			"conditions": conditions,
		})
	}

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"chart":       "decision_tree",
		"evaluations": story.decisionEvals,
		"nodes":       nodes,
		"recent":      story.recentDecisions,
	}})
}

func (story *Story) writePlaybookAudit(
	measurement types.Measurement,
	regime perspectives.RegimeFeatures,
	action reasoning.Action,
	trace reasoning.ReasonTrace,
) error {
	frame := map[string]any{
		"audit_event": "playbook_walk",
		"symbol":      measurement.Symbol,
		"source":      measurement.Source.String(),
		"category":    string(measurement.Category),
		"snr":         measurement.SNR,
		"price":       measurement.Last,
		"regime":      regime.Regime.String(),
		"verdict":     action.Type.String(),
		"side":        string(action.Side),
		"fraction":    action.Fraction,
		"offset":      action.Offset,
		"trace":       playbookAuditTrace(trace),
	}

	for key, value := range playbookAuditMeasurement(measurement) {
		frame[key] = value
	}

	return story.audit.Write(frame)
}

// positionState projects what the story knows about the open position into the
// view the reasoning language reasons over. Holding comes from the trader's
// exchange-reconciled inventory (paper/live holdings snapshots and fills), not
// the chart focus set. Entry, peak, and elapsed derive from prices observed
// since the position opened.
func (story *Story) positionState(measurement types.Measurement) reasoning.PositionState {
	symbol := measurement.Symbol
	holding := story.positionHeld != nil && story.positionHeld(symbol)

	now := measurement.At
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if !holding {
		delete(story.positions, symbol)

		return reasoning.PositionState{Holding: false, Last: measurement.Last, Now: now}
	}

	state, ok := story.positions[symbol]

	if !ok {
		state = &reasoning.PositionState{
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
func (story *Story) reasonState(symbol string) *reasoning.ReasonState {
	state, ok := story.reasonStates[symbol]

	if !ok {
		state = reasoning.NewReasonState()
		story.reasonStates[symbol] = state
	}

	return state
}

/*
publishMarketRegime sends the cross-section averaged regime radar to the dashboard.
Each symbol's axes contribute independently; zero axes are excluded from the mean.
*/
func (story *Story) publishMarketRegime() {
	if story.ui == nil || len(story.regimeFeatures) == 0 {
		return
	}

	perSymbol := make(map[string]map[string]float64, len(story.regimeFeatures))

	for symbol, features := range story.regimeFeatures {
		perSymbol[symbol] = features.Radar()
	}

	axes := perspectives.AverageRadarAxes(perSymbol)
	regime := perspectives.MajorityRegime(story.regimeFeatures)
	perspectives.PublishRegime(regime)

	story.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"chart":      "regime",
		"symbol":     marketSymbol,
		"regime":     regime.String(),
		"volatility": axes[perspectives.RegimeAxisVolatility],
		"trend":      axes[perspectives.RegimeAxisTrend],
		"bullish":    axes[perspectives.RegimeAxisBullish],
		"bearish":    axes[perspectives.RegimeAxisBearish],
		"choppiness": axes[perspectives.RegimeAxisChoppiness],
	}})
}

/*
rotateExistingCapture renames a non-empty capture aside with its mtime stamp so
`make run --record` appends history instead of destroying it. Tune still reads
the configured path (the freshest capture); older sessions stay on disk.
*/
func rotateExistingCapture(path string) {
	info, err := os.Stat(path)

	if err != nil || info.Size() == 0 {
		return
	}

	rotated := fmt.Sprintf(
		"%s.%s",
		path,
		info.ModTime().UTC().Format("20060102T150405Z"),
	)

	if err := os.Rename(path, rotated); err != nil {
		errnie.Error(fmt.Errorf("story: rotate capture %q: %w", path, err), "story")

		return
	}

	errnie.Info("rotated previous capture to "+rotated, "market/story")
}

// loadThoughts reads the reasoning playbook the optimizer writes. A missing file
// is not fatal: the story simply does not act until a playbook is tuned.
func (story *Story) loadThoughts() ([]reasoning.Thought, error) {
	if viper.GetBool("market.perspectives.fixture_playbook") {
		return perspectives.FixturePlaybook(), nil
	}

	path := playbookPath()

	raw, err := os.ReadFile(path)

	if err != nil {
		errnie.Info("no playbook at "+path+" — story idle until one is tuned", "market/story")

		return nil, nil
	}

	return reasoning.ParseThoughts(raw)
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
SetBookEnricher wires L2 depth attachment for capture recording. The enricher is
installed from cmd startup where the quote cache lives, avoiding an import cycle
between market and broker.
*/
func (story *Story) SetBookEnricher(
	enricher func(types.Measurement) types.Measurement,
) {
	story.bookEnricher = enricher
}

/*
SetQuoteReady gates playbook actions until the quote cache has a snapshot for the
symbol, so the trader does not race the ticker ingest and surface a false "no quote"
rejection on the dashboard.
*/
func (story *Story) SetQuoteReady(ready func(string) bool) {
	story.quoteReady = ready
}

/*
SetPositionHeld wires the trader's exchange-reconciled inventory view so exit
managers only arm when a symbol is actually held. The chart focus set is UI-only
and must not drive lifecycle predicates.
*/
func (story *Story) SetPositionHeld(held func(string) bool) {
	story.positionHeld = held
}

func (story *Story) stampQuoteNotional(measurement types.Measurement) types.Measurement {
	return StampQuoteNotional(measurement, story.quoteVolumeBase)
}

/*
StampQuoteNotional recomputes quote notional (24h base volume × last) on every
measurement so volume rose_by predicates track price-linked participation instead
of a stale cached notional copied from the last liquidity reading.
*/
func StampQuoteNotional(
	measurement types.Measurement, quoteVolumeBase map[string]float64,
) types.Measurement {
	if measurement.Last <= 0 {
		return measurement
	}

	if measurement.Volume > 0 {
		quoteVolumeBase[measurement.Symbol] = measurement.Volume / measurement.Last
	}

	baseVolume, ok := quoteVolumeBase[measurement.Symbol]
	if !ok || baseVolume <= 0 {
		return measurement
	}

	measurement.Volume = baseVolume * measurement.Last

	return measurement
}

func (story *Story) enrichMeasurementBook(
	measurement types.Measurement,
) types.Measurement {
	if story.bookEnricher == nil {
		return measurement
	}

	return story.bookEnricher(measurement)
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()

	if subscriber := story.subscribers["measurements"]; subscriber != nil {
		if broadcast := story.broadcasts["measurements"]; broadcast != nil {
			broadcast.Unsubscribe(storyMeasurementsSubscriberID)
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

	if story.audit != nil {
		closeErr = errors.Join(closeErr, story.audit.Close())
		story.audit = nil
	}

	return closeErr
}
