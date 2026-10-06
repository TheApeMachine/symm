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
	// excursionUp is the largest causal rise that clears round-trip friction.
	excursionUp = "up"
	// excursionUpShort is the largest causal rise that falls short of
	// round-trip friction: the near miss that looks like ignition but loses.
	excursionUpShort = "up_friction"
	// excursionDown is the largest causal fall whose magnitude clears
	// round-trip friction.
	excursionDown = "down"
	// excursionChop is the longest stretch whose whole price range stays
	// inside the friction deadband without being a flat line.
	excursionChop = "chop"
	// excursionFlat is the longest run of trades at one unchanged price.
	excursionFlat = "flat"
)

/*
Detector scans market tape from beginning to end.
Given raw spot trade measurements, the detector finds, for each symbol and
epoch, one representative fragment of every excursion class:

  - up: the best friction-clearing low -> high move;
  - up_friction: the best low -> high move that does not clear friction;
  - down: the best high -> low move whose magnitude clears friction;
  - chop: the longest deadband stretch (range never clears friction);
  - flat: the longest run of one unchanged price.

The scan is streaming and O(1) in memory per symbol/epoch tape.
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
tapePoint is one priced trade coordinate on the tape.
*/
type tapePoint struct {
	idx   int64
	tick  int64
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
improves reports whether a candidate ratio beats the best ratio so far.
Equal ratios prefer the wider fragment.
*/
func improves(ratio *big.Rat, candidate span, best *big.Rat, held span) bool {
	if best == nil {
		return true
	}

	cmp := ratio.Cmp(best)

	if cmp != 0 {
		return cmp > 0
	}

	return candidate.width() > held.width()
}

/*
tape is the streaming state for one contiguous symbol/epoch tape.
*/
type tape struct {
	detector *Detector
	epoch    int64
	symbol   string
	start    tapePoint
	// points keeps every observed trade so Flush can pad precursor left of B
	// and tape right of C (TRAINING.md) without re-reading storage.
	points []tapePoint

	// Running extremes. Equal prices never replace an extreme, so the
	// earliest occurrence gives the widest interval.
	trough tapePoint
	peak   tapePoint

	// Best completed excursions. They are independent of later changes to
	// the running extremes, so a late wick cannot erase an earlier move.
	up       span
	upGain   *big.Rat
	near     span
	nearGain *big.Rat
	down     span
	downDrop *big.Rat

	// clearGain is the smallest gross gain observed to clear friction.
	// Round-trip PnL sign depends only on the exit/entry ratio against the
	// fee, so any larger gain clears as well and need not be priced again.
	// The ratio threshold is fee-specific: clearFee is the fee rate it was
	// priced under, and a fee change (tier refresh) drops the cache.
	clearGain *big.Rat
	clearFee  *decimal.Decimal

	// Current deadband stretch and its price range.
	quiet    span
	quietMin *decimal.Decimal
	quietMax *decimal.Decimal
	chop     span

	// Current run of one unchanged price.
	run  span
	flat span
}

func (t *tape) observe(p tapePoint) error {
	t.points = append(t.points, p)

	if t.trough.price == nil {
		t.trough, t.peak = p, p
		t.quiet = span{b: p, c: p}
		t.quietMin, t.quietMax = p.price, p.price
		t.run = span{b: p, c: p}
		return nil
	}

	if err := t.rise(p); err != nil {
		return err
	}

	t.fall(p)

	if err := t.settle(p); err != nil {
		return err
	}

	t.hold(p)
	return nil
}

/*
rise tracks the best low -> high move from the running trough, and the best
low -> high move that does not clear friction.
*/
func (t *tape) rise(p tapePoint) error {
	if p.price.Cmp(t.trough.price) < 0 {
		t.trough = p
		return nil
	}

	if p.tick <= t.trough.tick || p.price.Cmp(t.trough.price) == 0 {
		return nil
	}

	gain := new(big.Rat).Quo(p.price.Rat(), t.trough.price.Rat())
	candidate := span{b: t.trough, c: p}

	if improves(gain, candidate, t.upGain, t.up) {
		t.upGain, t.up = gain, candidate
	}

	fee := t.detector.feeRate(t.symbol)

	if t.clearGain != nil && (fee == nil || t.clearFee == nil || fee.Cmp(t.clearFee) != 0) {
		t.clearGain, t.clearFee = nil, nil
	}

	if t.clearGain != nil && gain.Cmp(t.clearGain) >= 0 {
		return nil
	}

	if !improves(gain, candidate, t.nearGain, t.near) {
		return nil
	}

	clears, err := t.detector.clearFriction(t.trough.price, p.price, t.symbol)
	if err != nil {
		return err
	}

	if clears {
		t.clearGain, t.clearFee = gain, fee
		return nil
	}

	t.nearGain, t.near = gain, candidate
	return nil
}

/*
fall tracks the best high -> low move from the running peak.
*/
func (t *tape) fall(p tapePoint) {
	if p.price.Cmp(t.peak.price) > 0 {
		t.peak = p
		return
	}

	if p.tick <= t.peak.tick || p.price.Cmp(t.peak.price) == 0 {
		return
	}

	drop := new(big.Rat).Quo(t.peak.price.Rat(), p.price.Rat())
	candidate := span{b: t.peak, c: p}

	if improves(drop, candidate, t.downDrop, t.down) {
		t.downDrop, t.down = drop, candidate
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
		clears, err := t.detector.clearFriction(low, high, t.symbol)
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

	if t.quiet.width() > t.chop.width() {
		t.chop = t.quiet
	}
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
	if t.run.width() > t.flat.width() {
		t.flat = t.run
	}
}

/*
flush publishes one detection per class found on the completed tape.
Fragments without a precursor before B, or whose B→C run is too short to
leave enter/exit sweet spots, are dropped (TRAINING.md: pad left of B).
*/
func (t *tape) flush() error {
	t.closeQuiet()
	t.closeFlat()

	publish := func(class string, excursion span) {
		start, end, ok := t.padded(excursion)
		if !ok {
			return
		}
		t.detector.Flush(class, t.symbol, t.epoch, start, end, excursion)
	}

	if t.up.held() {
		clears, err := t.detector.clearFriction(t.up.b.price, t.up.c.price, t.symbol)
		if err != nil {
			return err
		}

		if clears {
			publish(excursionUp, t.up)
		}
	}

	if t.near.held() {
		publish(excursionUpShort, t.near)
	}

	if t.down.held() {
		clears, err := t.detector.clearFriction(t.down.c.price, t.down.b.price, t.symbol)
		if err != nil {
			return err
		}

		if clears {
			publish(excursionDown, t.down)
		}
	}

	if t.chop.held() {
		publish(excursionChop, t.chop)
	}

	if t.flat.held() {
		publish(excursionFlat, t.flat)
	}

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

	bIdx, cIdx := -1, -1
	for i, p := range t.points {
		if p.tick == excursion.b.tick && p.idx == excursion.b.idx {
			bIdx = i
		}
		if p.tick == excursion.c.tick && p.idx == excursion.c.idx {
			cIdx = i
		}
	}

	if bIdx < 0 || cIdx <= bIdx {
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
padWindow is the tick window rehearsal/chart read around B→C when a stored
detection's start is tight or missing end (TRAINING.md left/right pad). The
pad equals the move width so long moves keep a proportional precursor; short
B→C spans get a minimum left pad so ignition is not the first lit token
(otherwise there is no precursor to teach, and B sits on the chart's left edge).
*/
func padWindow(b, c int64) (lo, hi int64) {
	width := c - b
	if width < 1 {
		width = 1
	}

	left := width
	if left < minPrecursorPad {
		left = minPrecursorPad
	}

	return b - left, c + max(width/2, 1)
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

		metric, err := readMetric(measurement, "price")

		if err != nil || metric == nil || metric.Exact == nil || metric.Exact.Sign() <= 0 {
			return detector.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[detector] spot:trade without a positive exact price: %s epoch=%d tick=%d",
					measurement.Label, measurement.Epoch, measurement.Tick,
				),
				err,
			))
		}

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

	detector.storeTee.Push(
		data.NewPublication(
			measurement,
			nil,
		),
	)

	return measurement
}
