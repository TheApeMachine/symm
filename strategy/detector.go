package strategy

import (
	"context"
	"fmt"
	"iter"
	"math/big"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Excursion classes (TRAINING.md "Excursion Detection"). Each detection is
stored with its class under the "type" metadata key.
*/
const (
	// excursionUp is one friction-clearing rise: a zigzag up leg.
	excursionUp = "up"
	// excursionUpShort is one rise that failed before clearing round-trip
	// friction: the near miss that looks like ignition but loses.
	excursionUpShort = "up_friction"
	// excursionDown is one fall whose magnitude clears friction: a zigzag
	// down leg.
	excursionDown = "down"
	// excursionChop is one stretch whose whole price range stays inside the
	// friction deadband without being a flat line.
	excursionChop = "chop"
	// excursionFlat is one run of consecutive trades at one unchanged price.
	excursionFlat = "flat"
)

/*
Detector scans market tape from beginning to end and emits every excursion
of every class it finds in a symbol/epoch tape, so one long collection run
yields all of its episodes. Episode boundaries come from the friction
deadband itself (the round-trip taker fee), never from a tuned constant:

  - up/down are the legs of a zigzag on the deadband. A leg from an anchor
    (B) runs while it makes new extremes (C) and ends on the first trade that
    retraces from C by a move that would itself clear friction; that trade
    starts the opposite leg. A new low below an up leg's B is such a retrace,
    so it also ends the leg. Every emitted leg clears friction.
  - up_friction is a rise from the running low of a falling (or not yet
    directed) tape that never cleared friction and failed: it ends when price
    makes a new low below its B. A rise that clears instead becomes an up leg.
  - chop is every maximal stretch whose price range does not clear friction
    (and is not a single price); the trade that would clear starts the next.
  - flat is every maximal run of two or more trades at one price.

Only episodes whose end a later trade confirms are published (see flush).
The scan is streaming; it keeps the tape's trades for padding.
*/
type Detector struct {
	*runtime.System
	storeTee runtime.Tee
	price    *broker.Price
}

/*
NewDetector creates a new detector and stores the tee used to publish
detected excursions. Every excursion class is defined against round-trip
friction, so the price system is a hard dependency: without it the detector
enters ERROR and Scan refuses to classify anything.
*/
func NewDetector(
	ctx context.Context,
	storeTee runtime.Tee,
	price *broker.Price,
) *Detector {
	detector := &Detector{
		System:   runtime.NewSystem(ctx, "detector"),
		storeTee: storeTee,
		price:    price,
	}

	if err := errnie.Require(map[string]any{
		"price":    price,
		"storeTee": storeTee,
	}); err != nil {
		detector.Error(errnie.Err(
			errnie.Validation,
			"[detector] cannot classify excursions without friction",
			err,
		))
	}

	return detector
}

/*
tapePoint is one priced trade coordinate on the tape. pos is its index in
the tape's retained points.
*/
type tapePoint struct {
	idx   int64
	tick  int64
	pos   int
	at    time.Time
	price *decimal.Decimal
}

/*
span is a B -> C pair of tape points.
*/
type span struct {
	b tapePoint
	c tapePoint
}

func (s span) held() bool {
	return s.b.price != nil && s.c.price != nil && s.b.tick < s.c.tick
}

func (s span) width() int64 {
	if !s.held() {
		return 0
	}

	return s.c.tick - s.b.tick
}

/*
friction answers whether buying at low and selling at high clears round-trip
taker friction. The sign of the round trip depends only on high/low against
the fee, so the tightest ratios known to clear and to fail bound every later
question under the same fee without pricing it again; a fee change (tier
refresh) drops both bounds.
*/
type friction struct {
	detector *Detector
	symbol   string
	fee      *decimal.Decimal
	clears   *big.Rat
	fails    *big.Rat
}

func (f *friction) clear(low, high *decimal.Decimal) (bool, error) {
	if high.Cmp(low) <= 0 {
		return false, nil
	}

	fee := f.detector.feeRate(f.symbol)

	if fee == nil || f.fee == nil || fee.Cmp(f.fee) != 0 {
		f.fee, f.clears, f.fails = fee, nil, nil
	}

	ratio := new(big.Rat).Quo(high.Rat(), low.Rat())

	if f.clears != nil && ratio.Cmp(f.clears) >= 0 {
		return true, nil
	}

	if f.fails != nil && ratio.Cmp(f.fails) <= 0 {
		return false, nil
	}

	clears, err := f.detector.clearFriction(low, high, f.symbol)

	if err != nil {
		return false, err
	}

	if clears {
		f.clears = ratio
	} else {
		f.fails = ratio
	}

	return clears, nil
}

/*
leg directions of the zigzag.
*/
const (
	legNone = iota
	legUp
	legDown
)

/*
tape is the streaming state for one contiguous symbol/epoch tape.
*/
type tape struct {
	detector *Detector
	epoch    int64
	symbol   string
	start    tapePoint
	// points keeps every observed trade so flush can pad precursor left of B
	// and tape right of C (TRAINING.md) without re-reading storage.
	points   []tapePoint
	friction friction
	// found holds every completed episode in completion order.
	found []episode

	// Zigzag. In an up leg, low is its B and high its running maximum; in a
	// down leg, high is its B and low its running minimum; before the first
	// leg, both are the tape's running extremes. bounce is the highest trade
	// since low, the C of a rise that has not yet cleared. Equal prices never
	// replace an extreme, so the earliest occurrence is kept.
	leg    int
	low    tapePoint
	high   tapePoint
	bounce tapePoint

	// Current deadband stretch and its price range.
	quiet    span
	quietMin *decimal.Decimal
	quietMax *decimal.Decimal

	// Current run of one unchanged price.
	run span
}

/*
episode is one completed excursion and its class.
*/
type episode struct {
	class     string
	excursion span
}

func (t *tape) emit(class string, excursion span) {
	if excursion.held() {
		t.found = append(t.found, episode{class: class, excursion: excursion})
	}
}

func (t *tape) observe(p tapePoint) error {
	p.pos = len(t.points)
	t.points = append(t.points, p)

	if t.low.price == nil {
		t.friction = friction{detector: t.detector, symbol: t.symbol}
		t.low, t.high, t.bounce = p, p, p
		t.quiet = span{b: p, c: p}
		t.quietMin, t.quietMax = p.price, p.price
		t.run = span{b: p, c: p}
		return nil
	}

	if err := t.zigzag(p); err != nil {
		return err
	}

	if err := t.settle(p); err != nil {
		return err
	}

	t.hold(p)
	return nil
}

/*
zigzag advances the legs by one trade.
*/
func (t *tape) zigzag(p tapePoint) error {
	switch t.leg {
	case legUp:
		if p.price.Cmp(t.high.price) > 0 {
			t.high = p
			return nil
		}

		clears, err := t.friction.clear(p.price, t.high.price)

		if err != nil || !clears {
			return err
		}

		// The retrace from C clears friction: the up leg is over and this
		// trade opens the down leg from C.
		t.emit(excursionUp, span{b: t.low, c: t.high})
		t.leg = legDown
		t.low, t.bounce = p, p
		return nil

	default:
		if t.leg == legNone && p.price.Cmp(t.high.price) > 0 {
			t.high = p
		}

		if p.price.Cmp(t.low.price) < 0 {
			// A new low ends the rise from the old one: it never cleared, or
			// it would already have become an up leg.
			t.emit(excursionUpShort, span{b: t.low, c: t.bounce})
			t.low, t.bounce = p, p

			if t.leg == legNone {
				clears, err := t.friction.clear(p.price, t.high.price)

				if err != nil || !clears {
					return err
				}

				t.leg = legDown
			}

			return nil
		}

		if p.price.Cmp(t.bounce.price) > 0 {
			t.bounce = p
		}

		clears, err := t.friction.clear(t.low.price, p.price)

		if err != nil || !clears {
			return err
		}

		// The rise from the running low clears friction: the down leg (if
		// any) ends at that low and this trade opens the up leg from it.
		if t.leg == legDown {
			t.emit(excursionDown, span{b: t.high, c: t.low})
		}

		t.leg = legUp
		t.high = p
		return nil
	}
}

/*
settle grows the current deadband stretch until a trade widens its price
range far enough to clear friction; that trade starts the next stretch.
*/
func (t *tape) settle(p tapePoint) error {
	low, high := t.quietMin, t.quietMax
	widened := false

	if p.price.Cmp(low) < 0 {
		low, widened = p.price, true
	}

	if p.price.Cmp(high) > 0 {
		high, widened = p.price, true
	}

	if widened {
		clears, err := t.friction.clear(low, high)
		if err != nil {
			return err
		}

		if clears {
			t.closeQuiet()
			t.quiet = span{b: p, c: p}
			t.quietMin, t.quietMax = p.price, p.price
			return nil
		}
	}

	t.quiet.c = p
	t.quietMin, t.quietMax = low, high
	return nil
}

func (t *tape) closeQuiet() {
	if t.quietMin == nil || t.quietMin.Cmp(t.quietMax) == 0 {
		return
	}

	t.emit(excursionChop, t.quiet)
}

/*
hold grows the current run of one unchanged price.
*/
func (t *tape) hold(p tapePoint) {
	if p.price.Cmp(t.run.b.price) == 0 {
		t.run.c = p
		return
	}

	t.closeFlat()
	t.run = span{b: p, c: p}
}

func (t *tape) closeFlat() {
	t.emit(excursionFlat, t.run)
}

/*
flush publishes every episode of the completed tape whose end is confirmed by
a later trade: a leg ended by a friction-clearing retrace, a failed rise ended
by a new low, a chop stretch ended by the trade that cleared friction, a flat
run ended by a price change. Episodes still open at the end of the tape are
not published, so a detect run over an epoch that is still being collected
publishes only episodes the rest of the tape can never change, and a later run
over the longer tape reproduces every one of them.
*/
func (t *tape) flush() error {
	for _, found := range t.found {
		start, end, ok := t.padded(found.excursion)

		if !ok {
			continue
		}

		t.detector.Flush(found.class, t.symbol, t.epoch, start, end, found.excursion)
	}

	t.found = nil
	return nil
}

/*
padded answers the stored fragment window around an excursion: precursor left
of B at least as many trades as the B→C move, and some tape right of C
(TRAINING.md). ok is false when there is no trade before B or B→C is too
short to leave a frame between them.
*/
func (t *tape) padded(excursion span) (start, end tapePoint, ok bool) {
	if !excursion.held() || len(t.points) == 0 {
		return tapePoint{}, tapePoint{}, false
	}

	bIdx, cIdx := excursion.b.pos, excursion.c.pos

	if bIdx < 0 || cIdx <= bIdx || cIdx >= len(t.points) {
		return tapePoint{}, tapePoint{}, false
	}

	width := cIdx - bIdx
	// Pad scale follows the move; short B→C still stores — rehearsal teaches
	// those phases with weakened feedback rather than soft-skipping.
	left, right := max(width, 1), max(width/2, 1)
	startIdx := max(0, bIdx-left)
	endIdx := min(len(t.points)-1, cIdx+right)
	// startIdx may equal bIdx when B is the first trade of the scanned tape;
	// still store B→C with right pad. Rehearsal/chart padWindow reads left of
	// B from storage when earlier trades exist (TRAINING.md precursor).

	return t.points[startIdx], t.points[endIdx], true
}

/*
PadWindow is the tick window rehearsal/chart read around B→C when a stored
detection's start is tight or missing end (TRAINING.md left/right pad). The
pad equals the move width so long moves keep a proportional precursor; short
B→C spans get a minimum left pad so ignition is not the first lit token
(otherwise there is no precursor to teach, and B sits on the chart's left edge).
The left pad never goes before tick 0: when B is near the epoch start, lo is
clamped to 0 and hi is kept >= lo so SignalLogic/Timeline never see a negative
lowTick.
*/
func PadWindow(b, c int64) (lo, hi int64) {
	width := max(c-b, 1)
	left := max(width, minPrecursorPad)

	// Tick 0 is the earliest stored frame; never ask the catalog for a
	// negative lowTick (SignalLogic rejects it and halted training).
	lo = max(b-left, 0)
	hi = max(c+max(width/2, 1), lo)

	return lo, hi
}

// minPrecursorPad is the smallest left pad (in ticks) so short B→C fragments
// still carry teachable precursor frames before ignition.
const minPrecursorPad = 4

/*
Scan scans the tape once from beginning to end and flushes the detections of
every contiguous symbol/epoch tape when its boundary is crossed, and of the
last tape when the iterator is exhausted. Only the B/C coordinates of each
detection are retained; the native tape fragment is recovered from storage
by its epoch and tick coordinates.

Scan halts on the first error: a tape read failure, a detector without a
price system, a trade without a positive exact price, or a round trip that
cannot be priced. It never classifies an excursion as if friction were zero,
and never flushes a tape whose read failed part-way, because its last
excursion would be cut at an arbitrary tick instead of its real end.
*/
func (detector *Detector) Scan(
	measurements iter.Seq2[*data.Measurement, error],
) error {
	if err := detector.Error(); err != nil {
		return err
	}

	var current *tape

	for measurement, err := range measurements {
		if err != nil {
			return detector.Error(errnie.Err(
				errnie.IO,
				"[detector] trade tape read failed",
				err,
			))
		}

		if measurement == nil {
			continue
		}

		if measurement.Source != "spot:trade" {
			continue
		}

		if current != nil && (measurement.Label != current.symbol || measurement.Epoch != current.epoch) {
			if err := current.flush(); err != nil {
				return detector.Error(err)
			}

			current = nil
		}

		if current == nil {
			current = &tape{
				detector: detector,
				epoch:    measurement.Epoch,
				symbol:   measurement.Label,
				start: tapePoint{
					idx:  measurement.SeqIdx,
					tick: measurement.Tick,
					at:   measurement.At,
				},
			}
		}

		metric := data.Pull(measurement.Read("price")).Metric

		if measurement.At.IsZero() {
			return detector.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[detector] spot:trade without venue time (At): %s epoch=%d tick=%d",
					measurement.Label, measurement.Epoch, measurement.Tick,
				),
				nil,
			))
		}

		if metric == nil || metric.Exact == nil {
			return detector.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[detector] spot:trade without exact price: %s epoch=%d tick=%d",
					measurement.Label, measurement.Epoch, measurement.Tick,
				),
				nil,
			))
		}

		price := metric.Exact

		if err := current.observe(tapePoint{
			idx:   measurement.SeqIdx,
			tick:  measurement.Tick,
			at:    measurement.At,
			price: price,
		}); err != nil {
			return detector.Error(err)
		}
	}

	if current != nil {
		if err := current.flush(); err != nil {
			return detector.Error(err)
		}
	}

	return nil
}

/*
clearFriction verifies that buying at lowPrice and selling at highPrice
remains profitable after round-trip taker friction. A missing price system
or a round trip that cannot be priced is an error, never a guess.
*/
/*
feeRate returns the fee rate clearFriction prices under for symbol, or nil
when none is available (clearFriction then errors on its own).
*/
func (detector *Detector) feeRate(symbol string) *decimal.Decimal {
	if detector.price == nil {
		return nil
	}

	fee := detector.price.Fee(symbol)

	if fee == nil {
		return nil
	}

	return fee.Fee
}

func (detector *Detector) clearFriction(
	lowPrice *decimal.Decimal,
	highPrice *decimal.Decimal,
	symbol string,
) (bool, error) {
	if detector.price == nil {
		return false, errnie.Err(
			errnie.Validation,
			"[detector] price system is required to measure friction: "+symbol,
			nil,
		)
	}

	pnl, _, err := detector.price.RoundTrip(
		symbol,
		lowPrice,
		highPrice,
	)

	if err != nil {
		return false, errnie.Err(
			errnie.Validation,
			"[detector] round trip cannot be priced: "+symbol,
			err,
		)
	}

	if pnl == nil {
		return false, errnie.Err(
			errnie.Validation,
			"[detector] round trip returned no pnl: "+symbol,
			nil,
		)
	}

	return pnl.Sign() > 0, nil
}

/*
Flush writes one detected excursion of the given class to storeTee.
B is ignition (or the start of a chop/flat stretch) and C is exhaustion (or
its end). start and end are the padded fragment window (precursor left of B,
tape right of C); the stored epoch and tick coordinates identify that
contiguous historical tape.
*/
func (detector *Detector) Flush(
	class string,
	symbol string,
	epoch int64,
	start tapePoint,
	end tapePoint,
	excursion span,
) *data.Measurement {
	measurement := data.NewMeasurement(
		epoch,
		symbol,
		detector.Name(),
		start.idx,
		end.tick,
		&data.StringEntry{Key: "type", Value: class},
	)
	measurement.At = excursion.c.at
	measurement.From = excursion.b.at

	measurement.Write(
		data.NewMetric("start_idx", float64(start.idx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("start_tick", float64(start.tick), data.UnitCount, data.TimescaleTick),
		data.NewMetric("b_idx", float64(excursion.b.idx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("b_tick", float64(excursion.b.tick), data.UnitCount, data.TimescaleTick),
		data.NewMetric("c_idx", float64(excursion.c.idx), data.UnitCount, data.TimescaleTick),
		data.NewMetric("c_tick", float64(excursion.c.tick), data.UnitCount, data.TimescaleTick),
		data.NewMetric("end_tick", float64(end.tick), data.UnitCount, data.TimescaleTick),
		data.NewExactMetric("b_price", excursion.b.price, data.UnitPrice, data.TimescaleTick),
		data.NewExactMetric("c_price", excursion.c.price, data.UnitPrice, data.TimescaleTick),
	)

	detector.storeTee.Push(measurement)

	return measurement
}
