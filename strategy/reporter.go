package strategy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type StageCode int

const (
	StageModelDevelopment     StageCode = 0
	StageHistoricalValidation StageCode = 1
	StageForwardPaperLearning StageCode = 2
)

func (stage StageCode) String() string {
	switch stage {
	case StageModelDevelopment:
		return "MODEL DEVELOPMENT"
	case StageHistoricalValidation:
		return "HISTORICAL VALIDATION"
	case StageForwardPaperLearning:
		return "FORWARD PAPER LEARNING"
	default:
		return "MODEL DEVELOPMENT"
	}
}

type ReportSnapshot struct {
	Source       string
	Symbol       string
	SeqIdx       int64
	At           time.Time
	Stage        StageCode
	Blocker      string
	Action       int // 1=Enter, 2=Exit, 0=Wait
	Confidence   float64
	Contrast     float64
	Tokens       []byte
	RegionTokens [][]byte
	GridCells    int // Grid cells (unique metric labels) learned so far.
	GridRegions  int // Regions in the grid's last committed partition.
	MarkA        int64
	MarkB        int64
	MarkC        int64
	Price        float64
	ExcursionMag float64
	Direction    string
	Clears       bool
	Event        string
	Trading      bool
	Resolved     int64   // Round trips counted into WinRate and Edge.
	WinRate      float64 // Share of round trips with positive PnL.
	Edge         float64 // Mean round-trip return on entry cost.
}

type LearningReportData struct {
	Stage      string               `json:"stage"`
	Blocker    string               `json:"blocker"`
	Grid       GridReportData       `json:"grid"`
	Paper      PaperReportData      `json:"paper"`
	Excursions ExcursionsReportData `json:"excursions"`
	At         time.Time            `json:"at"`
}

type GridReportData struct {
	Cells   int `json:"cells"`
	Regions int `json:"regions"`
}

type PaperReportData struct {
	Trading  bool    `json:"trading"`
	Resolved int64   `json:"resolved"`
	WinRate  float64 `json:"win_rate"`
	Edge     float64 `json:"edge"`
}

type ExcursionsReportData struct {
	Up          int64 `json:"up"`
	Down        int64 `json:"down"`
	Chop        int64 `json:"chop"`
	Flat        int64 `json:"flat"`
	Unsupported int64 `json:"unsupported"`
}

/*
Reporter writes the canonical learning telemetry onto Training's output
measurement, feeding the Learning Dashboard and ForwardLearningViz.
*/
type Reporter struct {
	steps       atomic.Int64
	decisions   atomic.Int64
	fragUp      atomic.Int64
	fragUpFric  atomic.Int64
	fragDown    atomic.Int64
	fragChop    atomic.Int64
	fragFlat    atomic.Int64
	fragUnsup   atomic.Int64
	tee         runtime.Tee
	lastLogNano atomic.Int64
	lastStage   atomic.Int32
	lastBlocker atomic.Pointer[string]
	lastTrades  atomic.Int64
	latestSnap  atomic.Pointer[ReportSnapshot]
	logFile     *os.File
	logFileMu   sync.Mutex
}

func NewReporter() *Reporter {
	reporter := &Reporter{}
	reporter.lastStage.Store(-1)
	return reporter
}

/*
Log writes a structured timestamped record to training.log.
*/
func (reporter *Reporter) Log(format string, args ...any) {
	if reporter == nil {
		return
	}

	reporter.logFileMu.Lock()
	defer reporter.logFileMu.Unlock()

	if reporter.logFile == nil {
		file, err := os.OpenFile("training.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		reporter.logFile = file

		if fi, err := file.Stat(); err == nil && fi.Size() == 0 {
			file.WriteString(fmt.Sprintf("=== SYMM TRAINING SESSION START %s ===\n", time.Now().UTC().Format(time.RFC3339)))
		}
	}

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	line := timestamp + " " + fmt.Sprintf(format, args...) + "\n"
	reporter.logFile.WriteString(line)
}

/*
Close flushes and closes the training log file.
*/
func (reporter *Reporter) Close() error {
	if reporter == nil {
		return nil
	}

	reporter.logFileMu.Lock()
	defer reporter.logFileMu.Unlock()

	if reporter.logFile != nil {
		err := reporter.logFile.Close()
		reporter.logFile = nil
		return err
	}

	return nil
}

/*
SetTee attaches the UI tee every published report is pushed to. It is wired
once, before the pipeline starts.
*/
func (reporter *Reporter) SetTee(tee runtime.Tee) {
	reporter.tee = tee
}

/*
Publish populates out with the snapshot's telemetry and pushes it to the UI
tee, when one is attached. It also logs periodic and event-driven high-density
progress reports to give immediate visibility into the learning system.
*/
func (reporter *Reporter) Publish(
	out *data.Measurement,
	snapshot ReportSnapshot,
	extraMetrics ...*data.Metric,
) *data.Measurement {
	out = reporter.Populate(out, snapshot, extraMetrics...)

	if reporter.tee != nil {
		reporter.tee.Push(out)
	}

	snapCopy := snapshot
	reporter.latestSnap.Store(&snapCopy)

	nowNano := time.Now().UnixNano()
	lastLog := reporter.lastLogNano.Load()
	stageChanged := int32(snapshot.Stage) != reporter.lastStage.Swap(int32(snapshot.Stage))
	tradesChanged := snapshot.Resolved != reporter.lastTrades.Swap(snapshot.Resolved)

	blockerChanged := false
	prevBlocker := reporter.lastBlocker.Load()

	if prevBlocker == nil || *prevBlocker != snapshot.Blocker {
		blockerCopy := snapshot.Blocker
		reporter.lastBlocker.Store(&blockerCopy)
		blockerChanged = true
	}

	if stageChanged || blockerChanged || tradesChanged || (nowNano-lastLog) >= int64(10*time.Second) {
		reporter.lastLogNano.Store(nowNano)
		summary := reporter.Summary(snapshot)
		errnie.Info(summary)
		reporter.Log("%s", summary)
	}

	return out
}

/*
Summary renders a single-line high-density summary of the learning system.
*/
func (reporter *Reporter) Summary(snapshot ReportSnapshot) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("[learning] stage=%s", snapshot.Stage.String()))

	if snapshot.Blocker != "" && snapshot.Blocker != "—" {
		builder.WriteString(fmt.Sprintf(" blocker=%q", snapshot.Blocker))
	}

	if snapshot.GridCells > 0 || snapshot.GridRegions > 0 {
		builder.WriteString(fmt.Sprintf(" grid=[cells:%d regions:%d]", snapshot.GridCells, snapshot.GridRegions))
	}

	if snapshot.Trading || snapshot.Resolved > 0 {
		builder.WriteString(fmt.Sprintf(" paper=[trades:%d win_rate:%.1f%% edge:%+.2f%%]",
			snapshot.Resolved, snapshot.WinRate*100, snapshot.Edge*100,
		))
	}

	up := reporter.fragUp.Load() + reporter.fragUpFric.Load()
	down := reporter.fragDown.Load()
	chop := reporter.fragChop.Load()
	flat := reporter.fragFlat.Load()
	unsup := reporter.fragUnsup.Load()

	if up+down+chop+flat+unsup > 0 {
		builder.WriteString(fmt.Sprintf(" excursions=[up:%d down:%d chop:%d flat:%d unsup:%d]",
			up, down, chop, flat, unsup,
		))
	}

	return builder.String()
}

/*
LatestSummary returns the summary of the latest published snapshot.
*/
func (reporter *Reporter) LatestSummary() string {
	snap := reporter.latestSnap.Load()

	if snap == nil {
		return ""
	}

	return reporter.Summary(*snap)
}

/*
ReportData builds structured high-value report data for inspection endpoints.
*/
func (reporter *Reporter) ReportData() any {
	snap := reporter.latestSnap.Load()

	if snap == nil {
		return nil
	}

	up := reporter.fragUp.Load() + reporter.fragUpFric.Load()

	return LearningReportData{
		Stage:   snap.Stage.String(),
		Blocker: snap.Blocker,
		Grid: GridReportData{
			Cells:   snap.GridCells,
			Regions: snap.GridRegions,
		},
		Paper: PaperReportData{
			Trading:  snap.Trading,
			Resolved: snap.Resolved,
			WinRate:  snap.WinRate,
			Edge:     snap.Edge,
		},
		Excursions: ExcursionsReportData{
			Up:          up,
			Down:        reporter.fragDown.Load(),
			Chop:        reporter.fragChop.Load(),
			Flat:        reporter.fragFlat.Load(),
			Unsupported: reporter.fragUnsup.Load(),
		},
		At: snap.At,
	}
}

/*
RecordFragment tallies one completed tape fragment by its excursion class.
Unknown classes land in fragments_unsupported.
*/
func (reporter *Reporter) RecordFragment(class string) {
	switch class {
	case "up":
		reporter.fragUp.Add(1)
	case "up_friction":
		reporter.fragUpFric.Add(1)
	case "down":
		reporter.fragDown.Add(1)
	case "chop":
		reporter.fragChop.Add(1)
	case "flat":
		reporter.fragFlat.Add(1)
	default:
		reporter.fragUnsup.Add(1)
	}
}

/*
excursionTypeCode maps the five trained tape classes onto the numeric
excursion_type metric the learning UI reads when direction metadata is absent.
*/
func excursionTypeCode(direction string) float64 {
	switch direction {
	case "up":
		return 1
	case "down":
		return 2
	case "chop":
		return 3
	case "flat":
		return 4
	case "up_friction":
		return 5
	default:
		return 0
	}
}

/*
Metadata builds all canonical provenance and metadata key-value pairs
for the report snapshot.
*/
func (reporter *Reporter) Metadata(snapshot ReportSnapshot) []*data.StringEntry {
	var entries []*data.StringEntry

	if snapshot.Direction != "" {
		entries = append(entries, &data.StringEntry{Key: "excursion_direction", Value: snapshot.Direction})
	}

	blockerMessage := snapshot.Blocker

	if blockerMessage == "" {
		blockerMessage = "—"
	}

	entries = append(
		entries,
		&data.StringEntry{Key: "stage", Value: snapshot.Stage.String()},
		&data.StringEntry{Key: "stage_blocker", Value: blockerMessage},
	)

	var tokenParts []string

	if len(snapshot.RegionTokens) > 0 {
		for _, tokenBytes := range snapshot.RegionTokens {
			if len(tokenBytes) == 1 {
				tokenParts = append(tokenParts, strconv.Itoa(int(tokenBytes[0])))
			}

			if len(tokenBytes) > 1 {
				if tokenBytes[0] == 'R' {
					tokenParts = append(tokenParts, string(tokenBytes))
				}

				if tokenBytes[0] != 'R' {
					tokenParts = append(tokenParts, fmt.Sprintf("0x%x", tokenBytes))
				}
			}
		}
	}

	if len(snapshot.RegionTokens) == 0 && len(snapshot.Tokens) > 0 {
		for _, tokenByte := range snapshot.Tokens {
			if tokenByte != 0 {
				tokenParts = append(tokenParts, strconv.Itoa(int(tokenByte)))
			}
		}
	}

	if len(tokenParts) > 0 {
		serializedTokens := strings.Join(tokenParts, ",")
		entries = append(entries, &data.StringEntry{Key: "precursor_tokens", Value: serializedTokens})
	}

	if snapshot.MarkA > 0 {
		entries = append(entries, &data.StringEntry{Key: "excursion_start", Value: strconv.FormatInt(snapshot.MarkA, 10)})
	}

	if snapshot.MarkB > 0 {
		entries = append(entries, &data.StringEntry{Key: "excursion_ignition", Value: strconv.FormatInt(snapshot.MarkB, 10)})
	}

	if snapshot.MarkC > 0 {
		entries = append(entries, &data.StringEntry{Key: "excursion_exit", Value: strconv.FormatInt(snapshot.MarkC, 10)})
	}

	entries = append(
		entries,
		&data.StringEntry{Key: "excursion_clears", Value: fmt.Sprintf("%t", snapshot.Clears)},
	)

	if snapshot.Event != "" {
		entries = append(entries, &data.StringEntry{Key: "excursion_event", Value: snapshot.Event})
	}

	return entries
}

/*
Metrics constructs the telemetry metrics for the report snapshot.
*/
func (reporter *Reporter) Metrics(snapshot ReportSnapshot) []*data.Metric {
	stepCount := reporter.steps.Add(1)

	if snapshot.Action != 0 {
		reporter.decisions.Add(1)
	}

	stepFloat := float64(stepCount)
	decisionFloat := float64(reporter.decisions.Load())
	resolvedCount := float64(snapshot.Resolved)

	metrics := make([]*data.Metric, 0, 32)

	stepMetric := data.NewMetric("steps", stepFloat, data.UnitCount, data.TimescaleSession)
	stepMetric.Standardized = stepFloat
	metrics = append(metrics, stepMetric)

	decisionMetric := data.NewMetric("decisions", decisionFloat, data.UnitCount, data.TimescaleSession)
	decisionMetric.Standardized = decisionFloat
	metrics = append(metrics, decisionMetric)

	resolvedMetric := data.NewMetric("resolved", resolvedCount, data.UnitCount, data.TimescaleSession)
	resolvedMetric.Standardized = resolvedCount
	metrics = append(metrics, resolvedMetric)

	edgeSampleMetric := data.NewMetric("edge_sample_count", resolvedCount, data.UnitCount, data.TimescaleSession)
	edgeSampleMetric.Standardized = resolvedCount
	metrics = append(metrics, edgeSampleMetric)

	winRateMetric := data.NewMetric("win_rate", snapshot.WinRate, data.UnitProbability, data.TimescaleRollingWindow)
	winRateMetric.Standardized = snapshot.WinRate
	metrics = append(metrics, winRateMetric)

	accuracyMetric := data.NewMetric("accuracy", snapshot.WinRate, data.UnitProbability, data.TimescaleRollingWindow)
	accuracyMetric.Standardized = snapshot.WinRate
	metrics = append(metrics, accuracyMetric)

	edgeMetric := data.NewMetric("edge", snapshot.Edge, data.UnitPercent, data.TimescaleRollingWindow)
	edgeMetric.Standardized = snapshot.Edge
	metrics = append(metrics, edgeMetric)

	confidenceMetric := data.NewMetric("confidence", snapshot.Confidence, data.UnitConfidence, data.TimescaleRollingWindow)
	confidenceMetric.Standardized = snapshot.Confidence
	metrics = append(metrics, confidenceMetric)

	contrastMetric := data.NewMetric("contrast", snapshot.Contrast, data.UnitRatio, data.TimescaleRollingWindow)
	contrastMetric.Standardized = snapshot.Contrast
	metrics = append(metrics, contrastMetric)

	stageMetric := data.NewMetric("stage_code", float64(snapshot.Stage), data.UnitCount, data.TimescaleSession)
	stageMetric.Standardized = float64(snapshot.Stage)
	metrics = append(metrics, stageMetric)

	tradingValue := 0.0

	if snapshot.Trading {
		tradingValue = 1.0
	}

	tradingMetric := data.NewMetric("trading", tradingValue, data.UnitProbability, data.TimescaleInstantaneous)
	tradingMetric.Standardized = tradingValue
	metrics = append(metrics, tradingMetric)

	actionMetric := data.NewMetric("action", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
	actionMetric.Standardized = float64(snapshot.Action)
	metrics = append(metrics, actionMetric)

	frozenMetric := data.NewMetric("frozen_prediction", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
	frozenMetric.Standardized = float64(snapshot.Action)
	metrics = append(metrics, frozenMetric)

	if snapshot.Clears {
		delayedMetric := data.NewMetric("delayed_target", 1.0, data.UnitRatio, data.TimescaleTick)
		delayedMetric.Standardized = 1.0
		metrics = append(metrics, delayedMetric)
	}

	if !snapshot.Clears && snapshot.Action != 0 {
		delayedMetric := data.NewMetric("delayed_target", float64(snapshot.Action), data.UnitRatio, data.TimescaleTick)
		delayedMetric.Standardized = float64(snapshot.Action)
		metrics = append(metrics, delayedMetric)
	}

	if code := excursionTypeCode(snapshot.Direction); code > 0 {
		excursionMetric := data.NewMetric("excursion_type", code, data.UnitCount, data.TimescaleEvent)
		excursionMetric.Standardized = code
		metrics = append(metrics, excursionMetric)
	}

	fragmentCounters := []struct {
		label string
		count float64
	}{
		{"fragments_up", float64(reporter.fragUp.Load())},
		{"fragments_up_friction", float64(reporter.fragUpFric.Load())},
		{"fragments_down", float64(reporter.fragDown.Load())},
		{"fragments_chop", float64(reporter.fragChop.Load())},
		{"fragments_flat", float64(reporter.fragFlat.Load())},
		{"fragments_unsupported", float64(reporter.fragUnsup.Load())},
	}

	for _, counter := range fragmentCounters {
		metric := data.NewMetric(counter.label, counter.count, data.UnitCount, data.TimescaleSession)
		metric.Standardized = counter.count
		metrics = append(metrics, metric)
	}

	markAMetric := data.NewMetric("mark_a", float64(snapshot.MarkA), data.UnitCount, data.TimescaleEvent)
	markAMetric.Standardized = float64(snapshot.MarkA)
	metrics = append(metrics, markAMetric)

	markBMetric := data.NewMetric("mark_b", float64(snapshot.MarkB), data.UnitCount, data.TimescaleEvent)
	markBMetric.Standardized = float64(snapshot.MarkB)
	metrics = append(metrics, markBMetric)

	markCMetric := data.NewMetric("mark_c", float64(snapshot.MarkC), data.UnitCount, data.TimescaleEvent)
	markCMetric.Standardized = float64(snapshot.MarkC)
	metrics = append(metrics, markCMetric)

	tokenLength := len(snapshot.RegionTokens)

	if tokenLength == 0 {
		tokenLength = len(snapshot.Tokens)
	}

	gridCellsMetric := data.NewMetric("grid_cells", float64(snapshot.GridCells), data.UnitCount, data.TimescaleSession)
	gridCellsMetric.Standardized = float64(snapshot.GridCells)
	metrics = append(metrics, gridCellsMetric)

	gridRegionsMetric := data.NewMetric("grid_regions", float64(snapshot.GridRegions), data.UnitCount, data.TimescaleSession)
	gridRegionsMetric.Standardized = float64(snapshot.GridRegions)
	metrics = append(metrics, gridRegionsMetric)

	precursorMetric := data.NewMetric("precursor_length", float64(tokenLength), data.UnitCount, data.TimescaleEvent)
	precursorMetric.Standardized = float64(tokenLength)
	metrics = append(metrics, precursorMetric)

	if snapshot.Price > 0 {
		priceMetric := data.NewMetric("price", snapshot.Price, data.UnitPrice, data.TimescaleTick)
		priceMetric.Standardized = snapshot.Price
		metrics = append(metrics, priceMetric)
	}

	magMetric := data.NewMetric("excursion_mag", snapshot.ExcursionMag, data.UnitSpread, data.TimescaleEvent)
	magMetric.Standardized = snapshot.ExcursionMag
	metrics = append(metrics, magMetric)

	return metrics
}

/*
Populate writes all canonical learning and evaluation metrics onto the measurement,
finalizing it into WORM locked state.
*/
func (reporter *Reporter) Populate(
	out *data.Measurement,
	snapshot ReportSnapshot,
	extraMetrics ...*data.Metric,
) *data.Measurement {
	if out == nil {
		return nil
	}

	out.SeqIdx = snapshot.SeqIdx

	if snapshot.Symbol != "" {
		out.Label = snapshot.Symbol
	}

	if !snapshot.At.IsZero() {
		out.At = snapshot.At
	}

	metrics := reporter.Metrics(snapshot)

	if len(extraMetrics) > 0 {
		metrics = append(metrics, extraMetrics...)
	}

	return out.Write(metrics...)
}
