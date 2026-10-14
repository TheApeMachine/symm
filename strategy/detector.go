package strategy

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
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
	tapes    map[string]*tape
	steps    atomic.Int64
	emitted  atomic.Int64
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
		tapes:    make(map[string]*tape),
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
		return detector
	}

	errnie.Info(fmt.Sprintf(
		"[detector] excursion detector initialized with friction cash budget %s",
		detector.price.ReferenceCash().String(),
	))

	return detector
}

/*
tapePoint is one priced trade coordinate on the tape. pos is its index in
the tape's retained points. bid and ask are the executable quotes from the
trade measurement's stamped book, falling back to price when unstamped.
entryAsk and exitBid are the executable VWAPs walking the stamped book
to buy and sell the allocation budget, falling back to price when unstamped.
*/
type tapePoint struct {
	idx         int64
	tick        int64
	pos         int
	at          time.Time
	price       *decimal.Decimal
	bid         *decimal.Decimal
	ask         *decimal.Decimal
	entryAsk    *decimal.Decimal
	exitBid     *decimal.Decimal
	bidDepthQty *decimal.Decimal
	posQty      *decimal.Decimal
}

func (detector *Detector) computeBookExecution(
	symbol string,
	snapshot *broker.Snapshot,
) (entryAsk, exitBid, bidDepthQty, posQty *decimal.Decimal, err error) {
	budget := detector.price.ReferenceCash()

	if budget == nil || budget.Sign() <= 0 {
		return nil, nil, nil, nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[detector] positive reference cash required on price system",
			nil,
		))
	}

	budget = budget.SetScale(decimal.DefaultScale)

	if snapshot == nil {
		return nil, nil, nil, nil, nil
	}

	if len(snapshot.Bids) > 0 {
		depth := decimal.NewFromInt64(0)

		for _, lvl := range snapshot.Bids {
			if lvl.Quantity != nil && lvl.Quantity.Sign() > 0 {
				depth = depth.Add(lvl.Quantity)
			}
		}

		if depth.Sign() > 0 {
			bidDepthQty = depth
		}
	}

	if len(snapshot.Asks) > 0 {
		fee := detector.price.Fee(symbol)
		feeRate := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)

		if fee != nil && fee.Fee != nil && fee.Fee.Sign() > 0 {
			feeRate = fee.Fee.SetScale(decimal.DefaultScale).Div(decimal.NewFromInt64(100))
		}

		maxGross := budget.Div(decimal.NewFromInt64(1).SetScale(decimal.DefaultScale).Add(feeRate))
		remainingGross := maxGross
		accumQty := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		accumCost := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)

		for _, lvl := range snapshot.Asks {
			if lvl.Price == nil || lvl.Price.Sign() <= 0 || lvl.Quantity == nil || lvl.Quantity.Sign() <= 0 {
				continue
			}

			if remainingGross.Sign() <= 0 {
				break
			}

			lvlCost := lvl.Price.SetScale(decimal.DefaultScale).Mul(lvl.Quantity)

			if lvlCost.Cmp(remainingGross) <= 0 {
				accumQty = accumQty.Add(lvl.Quantity)
				accumCost = accumCost.Add(lvlCost)
				remainingGross = remainingGross.Sub(lvlCost)
				continue
			}

			partialQty := remainingGross.SetScale(decimal.DefaultScale).Div(lvl.Price)
			accumQty = accumQty.Add(partialQty)
			accumCost = accumCost.Add(remainingGross)
			remainingGross = decimal.NewFromInt64(0)
			break
		}

		if remainingGross.Sign() == 0 && accumQty.Sign() > 0 {
			entryAsk = accumCost.SetScale(decimal.DefaultScale).Div(accumQty)
			posQty = accumQty

			if normalizer := detector.price.Normalizer(); normalizer != nil {
				if formatted, normErr := normalizer.FormatPrice(symbol, entryAsk); normErr == nil && formatted != nil {
					entryAsk = formatted
				}

				if formatted, normErr := normalizer.FormatSize(symbol, accumQty); normErr == nil && formatted != nil {
					posQty = formatted
				}
			}
		}
	}

	if posQty != nil && posQty.Sign() > 0 && len(snapshot.Bids) > 0 {
		remainingQty := posQty.SetScale(decimal.DefaultScale)
		accumRevenue := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)
		filledQty := decimal.NewFromInt64(0).SetScale(decimal.DefaultScale)

		for _, lvl := range snapshot.Bids {
			if lvl.Price == nil || lvl.Price.Sign() <= 0 || lvl.Quantity == nil || lvl.Quantity.Sign() <= 0 {
				continue
			}

			if remainingQty.Sign() <= 0 {
				break
			}

			if lvl.Quantity.Cmp(remainingQty) <= 0 {
				lvlRevenue := lvl.Price.SetScale(decimal.DefaultScale).Mul(lvl.Quantity)
				accumRevenue = accumRevenue.Add(lvlRevenue)
				filledQty = filledQty.Add(lvl.Quantity)
				remainingQty = remainingQty.Sub(lvl.Quantity)
				continue
			}

			lvlRevenue := lvl.Price.SetScale(decimal.DefaultScale).Mul(remainingQty)
			accumRevenue = accumRevenue.Add(lvlRevenue)
			filledQty = filledQty.Add(remainingQty)
			remainingQty = decimal.NewFromInt64(0)
			break
		}

		if remainingQty.Sign() == 0 && filledQty.Sign() > 0 {
			exitBid = accumRevenue.SetScale(decimal.DefaultScale).Div(filledQty)

			if normalizer := detector.price.Normalizer(); normalizer != nil {
				if formatted, normErr := normalizer.FormatPrice(symbol, exitBid); normErr == nil && formatted != nil {
					exitBid = formatted
				}
			}
		}
	}

	return entryAsk, exitBid, bidDepthQty, posQty, nil
}

/*
span is a B -> C pair of tape points.
*/
type span struct {
	b tapePoint
	c tapePoint
}

func (target span) held() bool {
	return target.b.price != nil && target.c.price != nil && target.b.tick < target.c.tick
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
	points []tapePoint
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

func (tape *tape) emit(class string, excursion span) {
	if excursion.held() {
		tape.found = append(tape.found, episode{class: class, excursion: excursion})
	}
}

func (tape *tape) observe(point tapePoint) error {
	point.pos = len(tape.points)
	tape.points = append(tape.points, point)

	if tape.low.price == nil {
		tape.low, tape.high, tape.bounce = point, point, point
		tape.quiet = span{b: point, c: point}
		tape.quietMin, tape.quietMax = point.price, point.price
		tape.run = span{b: point, c: point}
		return nil
	}

	if err := tape.zigzag(point); err != nil {
		return err
	}

	if err := tape.settle(point); err != nil {
		return err
	}

	tape.hold(point)
	return nil
}

func (tape *tape) safeProfit(lowPoint, candidatePoint tapePoint) float64 {
	if lowPoint.entryAsk == nil || candidatePoint.exitBid == nil || lowPoint.entryAsk.Sign() <= 0 || candidatePoint.exitBid.Sign() <= 0 {
		return 0
	}

	if candidatePoint.exitBid.Cmp(lowPoint.entryAsk) <= 0 {
		return 0
	}

	if tape.detector == nil || tape.detector.price == nil {
		return 0
	}

	pnl, _, err := tape.detector.price.RoundTrip(tape.symbol, lowPoint.entryAsk, candidatePoint.exitBid)

	if err != nil || pnl == nil || pnl.Sign() <= 0 {
		return 0
	}

	netPnl := pnl.Float64()

	if candidatePoint.bidDepthQty == nil {
		return netPnl
	}

	if lowPoint.posQty == nil || candidatePoint.bidDepthQty.Cmp(lowPoint.posQty) <= 0 {
		return 0
	}

	return netPnl * candidatePoint.bidDepthQty.Sub(lowPoint.posQty).Div(candidatePoint.bidDepthQty).Float64()
}

/*
zigzag advances the legs by one trade.
*/
func (tape *tape) zigzag(point tapePoint) error {
	switch tape.leg {
	case legUp:
		candidateProfit := tape.safeProfit(tape.low, point)
		highProfit := tape.safeProfit(tape.low, tape.high)

		if candidateProfit > highProfit {
			tape.high = point
			return nil
		}

		if point.price.Cmp(tape.high.price) >= 0 {
			return nil
		}

		pnl, _, err := tape.detector.price.RoundTrip(tape.symbol, point.price, tape.high.price)

		if err != nil || pnl == nil || pnl.Sign() <= 0 {
			return err
		}

		// The retrace from C clears friction: the up leg is over and this
		// trade opens the down leg from C.
		tape.emit(excursionUp, span{b: tape.low, c: tape.high})
		tape.leg = legDown
		tape.low, tape.bounce = point, point
		return nil

	default:
		if tape.leg == legNone && point.price.Cmp(tape.high.price) > 0 {
			tape.high = point
		}

		if point.price.Cmp(tape.low.price) < 0 {
			// A new low ends the rise from the old one: it never cleared, or
			// it would already have become an up leg.
			tape.emit(excursionUpShort, span{b: tape.low, c: tape.bounce})
			tape.low, tape.bounce = point, point

			if tape.leg == legNone && point.price.Cmp(tape.high.price) < 0 {
				pnl, _, err := tape.detector.price.RoundTrip(tape.symbol, point.price, tape.high.price)

				if err != nil || pnl == nil || pnl.Sign() <= 0 {
					return err
				}

				tape.leg = legDown
			}

			return nil
		}

		candidateProfit := tape.safeProfit(tape.low, point)
		bounceProfit := tape.safeProfit(tape.low, tape.bounce)

		if candidateProfit > bounceProfit || (bounceProfit == 0 && candidateProfit == 0 && point.price.Cmp(tape.bounce.price) > 0) {
			tape.bounce = point
		}

		if tape.low.ask == nil || point.bid == nil || point.bid.Cmp(tape.low.ask) <= 0 {
			return nil
		}

		pnl, _, err := tape.detector.price.RoundTrip(tape.symbol, tape.low.ask, point.bid)

		if err != nil || pnl == nil || pnl.Sign() <= 0 {
			return err
		}

		// The rise from the running low clears friction: the down leg (if
		// any) ends at that low and this trade opens the up leg from it.
		if tape.leg == legDown {
			tape.emit(excursionDown, span{b: tape.high, c: tape.low})
		}

		tape.leg = legUp
		tape.high = tape.bounce

		if tape.safeProfit(tape.low, point) > tape.safeProfit(tape.low, tape.high) {
			tape.high = point
		}

		return nil
	}
}

/*
settle grows the current deadband stretch until a trade widens its price
range far enough to clear friction; that trade starts the next stretch.
*/
func (tape *tape) settle(point tapePoint) error {
	low, high := tape.quietMin, tape.quietMax
	widened := false

	if point.price.Cmp(low) < 0 {
		low, widened = point.price, true
	}

	if point.price.Cmp(high) > 0 {
		high, widened = point.price, true
	}

	if widened {
		pnl, _, err := tape.detector.price.RoundTrip(tape.symbol, low, high)

		if err != nil {
			return err
		}

		if pnl != nil && pnl.Sign() > 0 {
			tape.closeQuiet()
			tape.quiet = span{b: point, c: point}
			tape.quietMin, tape.quietMax = point.price, point.price
			return nil
		}
	}

	tape.quiet.c = point
	tape.quietMin, tape.quietMax = low, high
	return nil
}

func (tape *tape) closeQuiet() {
	if tape.quietMin == nil || tape.quietMin.Cmp(tape.quietMax) == 0 {
		return
	}

	tape.emit(excursionChop, tape.quiet)
}

/*
hold grows the current run of one unchanged price.
*/
func (tape *tape) hold(point tapePoint) {
	if point.price.Cmp(tape.run.b.price) == 0 {
		tape.run.c = point
		return
	}

	tape.closeFlat()
	tape.run = span{b: point, c: point}
}

func (tape *tape) closeFlat() {
	tape.emit(excursionFlat, tape.run)
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
func (tape *tape) flush() error {
	for _, found := range tape.found {
		start, end, ok := tape.padded(found.excursion)

		if !ok {
			continue
		}

		tape.detector.Flush(found.class, tape.symbol, tape.epoch, start, end, found.excursion)
	}

	tape.found = nil
	return nil
}

/*
padded answers the stored fragment window around an excursion: precursor left
of B at least as many trades as the B→C move, and some tape right of C
(TRAINING.md). ok is false when there is no trade before B or B→C is too
short to leave a frame between them.
*/
func (tape *tape) padded(excursion span) (start, end tapePoint, ok bool) {
	if !excursion.held() || len(tape.points) == 0 {
		return tapePoint{}, tapePoint{}, false
	}

	bIdx, cIdx := excursion.b.pos, excursion.c.pos

	if bIdx < 0 || cIdx <= bIdx || cIdx >= len(tape.points) {
		return tapePoint{}, tapePoint{}, false
	}

	width := cIdx - bIdx
	// Pad scale follows the move; short B→C still stores — rehearsal teaches
	// those phases with weakened feedback rather than soft-skipping.
	left, right := max(width, 1), max(width/2, 1)
	startIdx := max(0, bIdx-left)
	endIdx := min(len(tape.points)-1, cIdx+right)
	// startIdx may equal bIdx when B is the first trade of the scanned tape;
	// still store B→C with right pad. Rehearsal/chart padWindow reads left of
	// B from storage when earlier trades exist (TRAINING.md precursor).

	return tape.points[startIdx], tape.points[endIdx], true
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
func PadWindow(bTick, cTick int64) (lo, hi int64) {
	width := max(cTick-bTick, 1)
	left := max(width, minPrecursorPad)

	// Tick 0 is the earliest stored frame; never ask the catalog for a
	// negative lowTick (SignalLogic rejects it and halted training).
	lo = max(bTick-left, 0)
	hi = max(cTick+max(width/2, 1), lo)

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
func (detector *Detector) Step(
	measurement *data.Measurement,
) *data.Measurement {
	if detector.Error() != nil || measurement == nil {
		return nil
	}

	trade := measurement
	if measurement.Source == "runtime:join" {
		for _, peer := range measurement.Peers() {
			if peer != nil && peer.Source == "spot:trade" {
				trade = peer
				break
			}
		}
	}

	if trade.Source != "spot:trade" {
		return nil
	}

	current := detector.tapes[trade.Label]

	if current != nil && trade.Epoch != current.epoch {
		if err := current.flush(); err != nil {
			detector.Error(err)
			return nil
		}

		delete(detector.tapes, trade.Label)
		current = nil
	}

	if current == nil {
		current = &tape{
			detector: detector,
			epoch:    trade.Epoch,
			symbol:   trade.Label,
			start: tapePoint{
				idx:  trade.SeqIdx,
				tick: trade.Tick,
				at:   trade.At,
			},
		}
		detector.tapes[trade.Label] = current
	}

	priceEntry := data.Pull(trade.Read("price"))

	if trade.At.IsZero() {
		detector.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[detector] spot:trade without venue time (At): %s epoch=%d tick=%d",
				trade.Label, trade.Epoch, trade.Tick,
			),
			nil,
		))
		return nil
	}

	if priceEntry == nil || priceEntry.Metric == nil || priceEntry.Metric.Exact() == nil {
		detector.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[detector] spot:trade without exact price: %s epoch=%d tick=%d",
				trade.Label, trade.Epoch, trade.Tick,
			),
			nil,
		))
		return nil
	}

	price := priceEntry.Metric.Exact()
	var bid *decimal.Decimal

	if bidEntry := data.Pull(trade.Read("bid_price_0")); bidEntry != nil && bidEntry.Metric != nil && bidEntry.Metric.Exact() != nil {
		bid = bidEntry.Metric.Exact()
	}

	var ask *decimal.Decimal

	if askEntry := data.Pull(trade.Read("ask_price_0")); askEntry != nil && askEntry.Metric != nil && askEntry.Metric.Exact() != nil {
		ask = askEntry.Metric.Exact()
	}

	snapshot, err := broker.ReadSnapshot(trade)

	if err != nil {
		detector.Error(err)
		return nil
	}

	if bid == nil && snapshot != nil && len(snapshot.Bids) > 0 {
		bid = snapshot.Bids[0].Price
	}

	if ask == nil && snapshot != nil && len(snapshot.Asks) > 0 {
		ask = snapshot.Asks[0].Price
	}

	entryAsk, exitBid, bidDepthQty, posQty, err := detector.computeBookExecution(trade.Label, snapshot)

	if err != nil {
		detector.Error(err)
		return nil
	}

	if err := current.observe(tapePoint{
		idx:         trade.SeqIdx,
		tick:        trade.Tick,
		at:          trade.At,
		price:       price,
		bid:         bid,
		ask:         ask,
		entryAsk:    entryAsk,
		exitBid:     exitBid,
		bidDepthQty: bidDepthQty,
		posQty:      posQty,
	}); err != nil {
		detector.Error(err)
		return nil
	}

	stepCount := detector.steps.Add(1)

	if stepCount == 1 || stepCount%500 == 0 {
		errnie.Info(fmt.Sprintf(
			"[detector] processing trades: %d observed (latest: %s tick=%d price=%s, active tapes: %d)",
			stepCount, trade.Label, trade.Tick, price.String(), len(detector.tapes),
		))
	}

	return nil
}

func (detector *Detector) FlushAll() error {
	totalTapes := len(detector.tapes)

	for symbol, current := range detector.tapes {
		if current != nil {
			if err := current.flush(); err != nil {
				return detector.Error(err)
			}
		}
		delete(detector.tapes, symbol)
	}

	errnie.Info(fmt.Sprintf(
		"[detector] flush complete: %d total trades processed, %d total excursions emitted across %d tape(s)",
		detector.steps.Load(), detector.emitted.Load(), totalTapes,
	))

	return nil
}

func (detector *Detector) Steps() int64 {
	return detector.steps.Load()
}

func (detector *Detector) Emitted() int64 {
	return detector.emitted.Load()
}

func (detector *Detector) Scan(
	measurements iter.Seq2[*data.Measurement, error],
) error {
	if err := detector.Error(); err != nil {
		return err
	}

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

		detector.Step(measurement)
	}

	return detector.FlushAll()
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
	var bPrice, cPrice *decimal.Decimal

	if class == excursionUp {
		bPrice = excursion.b.entryAsk
		cPrice = excursion.c.exitBid
	}

	if class != excursionUp {
		bPrice = excursion.b.price
		cPrice = excursion.c.price
	}

	if bPrice == nil || cPrice == nil || bPrice.Sign() <= 0 || cPrice.Sign() <= 0 {
		return nil
	}

	measurement := data.NewMeasurement(
		epoch,
		symbol,
		detector.Name(),
		excursion.c.at,
		excursion.b.at,
		&data.StringEntry{Key: "type", Value: class},
	)

	measurement.Tick = end.tick

	if measurement.Tick <= 0 {
		measurement.Tick = excursion.c.tick
	}

	measurement.SeqIdx = end.idx

	if measurement.SeqIdx <= 0 {
		measurement.SeqIdx = excursion.c.idx
	}

	if !end.at.IsZero() {
		measurement.Timestamp = end.at.UnixNano()
	}

	if end.at.IsZero() && !excursion.c.at.IsZero() {
		measurement.Timestamp = excursion.c.at.UnixNano()
	}

	startIdxUnit, startIdxTime := data.GetDimension(detector.Name(), "start_idx")
	startTickUnit, startTickTime := data.GetDimension(detector.Name(), "start_tick")
	bIdxUnit, bIdxTime := data.GetDimension(detector.Name(), "b_idx")
	bTickUnit, bTickTime := data.GetDimension(detector.Name(), "b_tick")
	cIdxUnit, cIdxTime := data.GetDimension(detector.Name(), "c_idx")
	cTickUnit, cTickTime := data.GetDimension(detector.Name(), "c_tick")
	endTickUnit, endTickTime := data.GetDimension(detector.Name(), "end_tick")
	bPriceUnit, bPriceTime := data.GetDimension(detector.Name(), "b_price")
	cPriceUnit, cPriceTime := data.GetDimension(detector.Name(), "c_price")

	measurement.Write(
		data.NewMetric("start_idx", float64(start.idx), startIdxUnit, startIdxTime),
		data.NewMetric("start_tick", float64(start.tick), startTickUnit, startTickTime),
		data.NewMetric("b_idx", float64(excursion.b.idx), bIdxUnit, bIdxTime),
		data.NewMetric("b_tick", float64(excursion.b.tick), bTickUnit, bTickTime),
		data.NewMetric("c_idx", float64(excursion.c.idx), cIdxUnit, cIdxTime),
		data.NewMetric("c_tick", float64(excursion.c.tick), cTickUnit, cTickTime),
		data.NewMetric("end_tick", float64(end.tick), endTickUnit, endTickTime),
		data.NewExactMetric("b_price", bPrice, bPriceUnit, bPriceTime),
		data.NewExactMetric("c_price", cPrice, cPriceUnit, cPriceTime),
	)

	detector.emitted.Add(1)
	errnie.Info(fmt.Sprintf(
		"[detector] detected %s on %s (epoch=%d, span=[tick %d -> %d], bPrice=%s, cPrice=%s)",
		class, symbol, epoch, excursion.b.tick, excursion.c.tick, bPrice.String(), cPrice.String(),
	))

	detector.storeTee.Push(measurement)

	return measurement
}
