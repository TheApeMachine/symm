package strategy

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/ui"
)

var _ ui.FragmentsSource = (*Chart)(nil)

/*
Chart owns the learned fragments the UI draws (TRAINING.md: the tape fragment
line, the A, B, and C markers, and the ENTER and EXIT prediction markers). It
reads each fragment's price tape, places its markers, keeps the fragment list,
and streams the fragment's report. Rehearsal only hands it what a fragment
taught.
*/
type Chart struct {
	catalog   *tables.Catalog
	reporter  *Reporter
	source    string
	fragments atomic.Pointer[[]ui.TrainedFragment]
	count     atomic.Int64
}

func newChart(catalog *tables.Catalog, reporter *Reporter, source string) *Chart {
	chart := &Chart{
		catalog:  catalog,
		reporter: reporter,
		source:   source,
	}

	empty := make([]ui.TrainedFragment, 0)
	chart.fragments.Store(&empty)
	return chart
}

/*
Fragments answers a copy of every fragment learned so far.
*/
func (chart *Chart) Fragments() []ui.TrainedFragment {
	return slices.Clone(*chart.fragments.Load())
}

/*
frames is the token tape a fragment was learned from and where its phases put
A, the ground-truth ENTER/EXIT sweet spots, and the post-teach predicted
ENTER/EXIT frames (Recall on the same contexts, no teach). A sweet spot or
prediction of -1 means that action was not taught or not predicted.
*/
type frames struct {
	ticks          []int64
	tokens         [][]byte
	a              int
	enter          int
	exit           int
	predictedEnter int
	predictedExit  int
}

/*
draw reads the fragment's price tape (padded window around B→C), places its
markers on the trades an order on those frames would fill against, and
records and reports it. Requires MarkA < MarkB < MarkC.
*/
func (chart *Chart) draw(
	ctx context.Context, detection *data.Measurement, move excursion, tape frames,
) error {
	lo, hi, err := move.window()

	if err != nil {
		return err
	}

	points, err := chart.priceTape(ctx, detection, lo, hi)

	if err != nil {
		return err
	}

	if tape.a < 0 || tape.a >= len(tape.ticks) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[chart] fragment A frame out of range: "+detection.Label,
			nil,
		))
	}

	markA := tape.ticks[tape.a]

	if !(markA < move.b && move.b < move.c) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[chart] markers must satisfy A < B < C: A=%d B=%d C=%d", markA, move.b, move.c),
			nil,
		))
	}

	direction := "flat"

	if move.gross > 0 {
		direction = "up"
	}

	if move.gross < 0 {
		direction = "down"
	}

	tokens := make([]string, len(tape.tokens))

	for index, token := range tape.tokens {
		tokens[index] = string(token)
	}

	return chart.publish(detection, ui.TrainedFragment{
		ID:                int(chart.count.Add(1)),
		Symbol:            detection.Label,
		Epoch:             detection.Epoch,
		MarkA:             markA,
		MarkB:             move.b,
		MarkC:             move.c,
		EntryPrice:        move.bPrice.Float64(),
		ExitPrice:         move.cPrice.Float64(),
		Magnitude:         move.gross,
		Direction:         direction,
		Class:             move.class,
		Tokens:            tokens,
		Points:            points,
		EntryIdx:          pointAt(points, tape.ticks, tape.enter),
		ExitIdx:           pointAt(points, tape.ticks, tape.exit),
		PredictedEntryIdx: pointAt(points, tape.ticks, tape.predictedEnter),
		PredictedExitIdx:  pointAt(points, tape.ticks, tape.predictedExit),
		LearnedAt:         time.Now(),
	})
}

/*
priceTape reads the fragment's chart points: the spot:trade rows of the
detection's label and epoch whose tick lies in [startTick, highTick], in tick
order (sequence index breaks ties), so X is the trade's position in tick order
and pointAt can binary-search the points by tick. FragmentPoint.Tick carries
the trade's tick.

Rows from any other source are ignored even when they carry a price metric,
and so are rows the Timeline admits only through its sequence-index bound.
Nothing is ever fabricated: a failed read, a trade without a positive exact
price, a trade without a timestamp, or a window holding no trade at all is an
error. The detector derives every excursion from this same trade tape, so an
empty window means storage and detections disagree.
*/
func (chart *Chart) priceTape(
	ctx context.Context, detection *data.Measurement, startTick, highTick int64,
) ([]ui.FragmentPoint, error) {
	startTick, highTick, err := clampTapeTicks(startTick, highTick)

	if err != nil {
		return nil, err
	}

	window := fmt.Sprintf(
		"%s epoch %d ticks %d..%d", detection.Label, detection.Epoch, startTick, highTick,
	)

	var trades []*data.Measurement

	for measurement, err := range chart.catalog.Timeline(
		ctx, detection.Epoch, detection.Label, startTick, highTick, "spot:trade",
	) {
		if err != nil {
			return nil, errnie.Err(errnie.IO, "[chart] unable to read price tape: "+window, err)
		}

		if measurement == nil || measurement.Source != "spot:trade" {
			continue
		}

		// Timeline already bounds on tick; re-check so the fragment's
		// window never depends on the catalog's filtering.
		if measurement.Label != detection.Label ||
			measurement.Epoch != detection.Epoch ||
			measurement.Tick < startTick ||
			measurement.Tick > highTick {
			continue
		}

		trades = append(trades, measurement)
	}

	if len(trades) == 0 {
		return nil, errnie.Err(
			errnie.NotFound, "[chart] detection has no spot:trade price tape: "+window, nil,
		)
	}

	// Timeline yields sequence-index order; pointAt searches by tick.
	slices.SortStableFunc(trades, func(left, right *data.Measurement) int {
		if order := cmp.Compare(left.Tick, right.Tick); order != 0 {
			return order
		}

		return cmp.Compare(left.SeqIdx, right.SeqIdx)
	})

	points := make([]ui.FragmentPoint, 0, len(trades))

	for _, trade := range trades {
		metric, err := readMetric(trade, "price")

		if err != nil || metric == nil || metric.Exact == nil || metric.Exact.Sign() <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[chart] spot:trade without a positive exact price in price tape: %s (tick %d seq %d)",
					window, trade.Tick, trade.SeqIdx,
				),
				err,
			)
		}

		timeMs := trade.At.UnixMilli()

		if timeMs <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[chart] spot:trade without a timestamp in price tape: %s (tick %d seq %d)",
					window, trade.Tick, trade.SeqIdx,
				),
				nil,
			)
		}

		points = append(points, ui.FragmentPoint{
			X:    len(points),
			Y:    metric.Exact.Float64(),
			Tick: trade.Tick,
			Time: timeMs,
		})
	}

	return points, nil
}

/*
pointAt maps a token frame index onto the fragment's price tape: the first
trade at or after the frame's tick, which is where an order placed on that
frame would fill. A negative frame, or a frame after the last trade, has no
marker and maps to -1.
*/
func pointAt(points []ui.FragmentPoint, ticks []int64, frame int) int {
	if frame < 0 || frame >= len(ticks) {
		return -1
	}

	index, _ := slices.BinarySearchFunc(points, ticks[frame], func(point ui.FragmentPoint, tick int64) int {
		return cmp.Compare(point.Tick, tick)
	})

	if index >= len(points) {
		return -1
	}

	return points[index].X
}

/*
publish records the learned fragment for the UI list and streams its report.
The fragment is historical: its venue time is the detection's C, never the
wall clock at replay, so a detection without venue time is an error.
*/
func (chart *Chart) publish(detection *data.Measurement, fragment ui.TrainedFragment) error {
	if detection.At.IsZero() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[chart] detection without venue time (At): %s tick %d",
				detection.Label, detection.Tick,
			),
			nil,
		))
	}

	for {
		previous := chart.fragments.Load()
		next := append(slices.Clip(*previous), fragment)

		if chart.fragments.CompareAndSwap(previous, &next) {
			break
		}
	}

	chart.reporter.RecordFragment(fragment.Class)

	var regionTokens [][]byte

	for _, token := range fragment.Tokens {
		regionTokens = append(regionTokens, []byte(token))
	}

	enters := fragment.EntryIdx >= 0 && fragment.EntryIdx < len(fragment.Points)
	action := 0

	if enters {
		action = 1
	}

	snapshot := ReportSnapshot{
		Source:       chart.source,
		Symbol:       detection.Label,
		SeqIdx:       fragment.MarkC,
		At:           detection.At,
		Stage:        StageHistoricalValidation,
		Blocker:      "historical validation",
		Action:       action,
		Confidence:   1.0,
		RegionTokens: regionTokens,
		MarkA:        fragment.MarkA,
		MarkB:        fragment.MarkB,
		MarkC:        fragment.MarkC,
		Price:        fragment.ExitPrice,
		ExcursionMag: fragment.Magnitude,
		Direction:    fragment.Class,
		Clears:       fragment.Class == excursionUp,
		Event:        "completed",
	}

	// Ground-truth ENTER/EXIT sit on teach sweet spots; predicted_* track the
	// post-teach Recall on the same contexts. Wait-only classes have none.
	var markers []*data.Metric

	if enters {
		mark := float64(fragment.Points[fragment.EntryIdx].Tick)
		metric := data.NewMetric("agent_entry", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	if fragment.ExitIdx >= 0 && fragment.ExitIdx < len(fragment.Points) {
		mark := float64(fragment.Points[fragment.ExitIdx].Tick)
		metric := data.NewMetric("agent_exit", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	if fragment.PredictedEntryIdx >= 0 && fragment.PredictedEntryIdx < len(fragment.Points) {
		mark := float64(fragment.Points[fragment.PredictedEntryIdx].Tick)
		metric := data.NewMetric("predicted_entry", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	if fragment.PredictedExitIdx >= 0 && fragment.PredictedExitIdx < len(fragment.Points) {
		mark := float64(fragment.Points[fragment.PredictedExitIdx].Tick)
		metric := data.NewMetric("predicted_exit", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	out := data.NewMeasurement(
		detection.Epoch,
		detection.Label,
		chart.source,
		fragment.MarkC,
		fragment.MarkC,
		chart.reporter.Metadata(snapshot)...,
	)
	out.At = snapshot.At
	out.From = snapshot.At

	chart.reporter.Publish(out, snapshot, markers...)
	return nil
}
