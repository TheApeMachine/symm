package strategy

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/ui"
)

/*
fragmentLog holds the stored detections Train read this session, as the
historical runs panel lists them: one per detection of any class, with its
A (tape start), B and C ticks and prices, and the precursor tokens Train cut
for an up excursion. Price points are read from the stored trade tape when a
fragment is opened, never kept for all of them.
*/
type fragmentLog struct {
	mu        sync.Mutex
	fragments []ui.TrainedFragment
	tapes     map[int]fragmentTape
}

/*
fragmentTape is where a fragment's trades live in the store.
*/
type fragmentTape struct {
	epoch int64
	label string
	low   int64
	high  int64
}

/*
add records one detection. tokens are the stored enter path's tokens
(without the action), empty when none were stored.
*/
func (log *fragmentLog) add(excursion *data.Measurement, tokens []string) {
	raw := func(key string) (float64, bool) {
		entry := data.Pull(excursion.Read(key))

		if entry == nil || entry.Err != nil || entry.Metric == nil {
			return 0, false
		}

		return entry.Metric.Raw, true
	}

	start, okA := raw("start_tick")
	b, okB := raw("b_tick")
	c, okC := raw("c_tick")

	if !okA || !okB || !okC {
		return
	}

	fragment := ui.TrainedFragment{
		Symbol:            excursion.Label,
		Epoch:             excursion.Epoch,
		MarkA:             int64(start),
		MarkB:             int64(b),
		MarkC:             int64(c),
		EntryIdx:          int(b),
		ExitIdx:           int(c),
		PredictedEntryIdx: -1,
		PredictedExitIdx:  -1,
		Class:             excursion.Meta("type"),
		Direction:         excursion.Meta("type"),
		Tokens:            tokens,
		Points:            []ui.FragmentPoint{},
		LearnedAt:         time.Now().UTC(),
	}

	if fragment.Tokens == nil {
		fragment.Tokens = []string{}
	}

	if bPrice, cPrice, err := tables.DetectionPrices(excursion); err == nil {
		fragment.EntryPrice, fragment.ExitPrice = bPrice.Float64(), cPrice.Float64()
		fragment.Magnitude = fragment.ExitPrice/fragment.EntryPrice - 1
	}

	log.mu.Lock()
	defer log.mu.Unlock()

	if log.tapes == nil {
		log.tapes = make(map[int]fragmentTape)
	}

	fragment.ID = len(log.fragments) + 1
	log.fragments = append(log.fragments, fragment)
	log.tapes[fragment.ID] = fragmentTape{
		epoch: excursion.Epoch, label: excursion.Label, low: int64(start), high: int64(c),
	}
}

/*
Fragments lists every detection Train has read this session, without points.
*/
func (training *Training) Fragments() []ui.TrainedFragment {
	training.fragments.mu.Lock()
	defer training.fragments.mu.Unlock()

	return slices.Clone(training.fragments.fragments)
}

/*
FragmentPoints reads one fragment's stored trades from A through C, one point
per trade: x its index, y its exact price, seq its market tick, time its venue
time in nanoseconds. A read error is returned, never an empty tape.
*/
func (training *Training) FragmentPoints(id int) ([]ui.FragmentPoint, error) {
	training.fragments.mu.Lock()
	tape, ok := training.fragments.tapes[id]
	training.fragments.mu.Unlock()

	if !ok {
		return nil, errnie.Err(errnie.NotFound, "[training] no fragment with that id", nil)
	}

	ctx := context.Background()

	if training.System != nil {
		ctx = training.Context()
	}

	trades := make([]*data.Measurement, 0)

	for trade, err := range training.catalog.Timeline(ctx, tape.epoch, tape.label, tape.low, tape.high, "spot:trade") {
		if err != nil {
			return nil, errnie.Err(errnie.IO, "[training] fragment tape read failed", err)
		}

		trades = append(trades, trade)
	}

	slices.SortStableFunc(trades, func(left, right *data.Measurement) int {
		if left.Tick != right.Tick {
			return int(left.Tick - right.Tick)
		}

		return int(left.SeqIdx - right.SeqIdx)
	})

	points := make([]ui.FragmentPoint, 0, len(trades))

	for _, trade := range trades {
		entry := data.Pull(trade.Read("price"))

		if entry == nil || entry.Err != nil || entry.Metric == nil || entry.Metric.Exact == nil {
			continue
		}

		points = append(points, ui.FragmentPoint{
			X:    len(points),
			Y:    entry.Metric.Exact.Float64(),
			Tick: trade.Tick,
			Time: trade.At.UnixNano(),
		})
	}

	return points, nil
}
