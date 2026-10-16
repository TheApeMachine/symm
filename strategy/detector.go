package strategy

import (
	"context"
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type detectorState int

const (
	stateInit detectorState = iota
	stateUndecided
	stateUp
	stateDown
)

type tradePoint struct {
	tick   int64
	seqIdx int64
	at     int64
	price  *decimal.Decimal
	priceF float64
	bid    *decimal.Decimal
	ask    *decimal.Decimal
}

type episode struct {
	class  string
	start  tradePoint // B: chronological start / ignition
	end    tradePoint // C: chronological end / exhaustion
	lowP   float64
	highP  float64
	trades int64
}

/*
ExcursionClasses are the classes a Detector emits; each is the Role of its
excursion Values, keyed data.Prefix("detector", class, symbol, epoch).
*/
var ExcursionClasses = []string{"up", "down", "chop", "flat"}

/*
ExcursionPrefixes are the keys of every excursion detected for symbol in
epoch, one prefix per class.
*/
func ExcursionPrefixes(symbol string, epoch uint64) []string {
	prefixes := make([]string, 0, len(ExcursionClasses))

	for _, class := range ExcursionClasses {
		prefixes = append(prefixes, data.Prefix(detectorOrigin, class, symbol, epoch))
	}

	return prefixes
}

const detectorOrigin = "detector"

/*
ExcursionKeys are the vector labels of an excursion Value, in order. B and C
are the ignition and exhaustion trades; A and D bound the precursor and
aftermath windows by half the move on either side. b_ask and c_bid are the
touch the tick recorded at B and C: the prices a long entry and exit cross.
*/
var ExcursionKeys = []string{
	"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
	"b_price", "c_price", "b_ask", "c_bid", "b_at", "c_at",
}

/*
Detector scans one symbol's stored trade tape of one epoch from beginning to
end and emits long, coherent excursions ("up", "down", "chop", "flat").
*/
type Detector struct {
	*runtime.System
	price  *broker.Price
	symbol string
	epoch  uint64

	// roundTrip is the symbol's round-trip break-even fraction (1+f)/(1-f)-1
	// at its taker fee f.
	roundTrip float64

	// Structural & regime-coalescing parameters
	retraceRatio           float64
	maxContinuationRetrace float64
	boxOverlapRatio        float64
	minFlatTrades          int64
	maxFlatGlitchTrades    int64
	minEpisodeTrades       int64
	minStagnationTrades    int64
	stagnationMultiplier   int64

	// Streaming state
	seq       int64
	state     detectorState
	emaBounce float64
	prevTrade *tradePoint
	prevDelta float64
	results   []*data.Value
	failure   error

	// Undecided / chop / flat tracking
	segStart      tradePoint
	runLow        tradePoint
	runHigh       tradePoint
	flatStart     tradePoint
	flatLast      tradePoint
	flatMin       float64
	flatMax       float64
	flatCount     int64
	flatGlitchCnt int64

	// Active directional leg (stateUp / stateDown)
	legB         tradePoint
	legC         tradePoint
	legExtreme   tradePoint
	legTrades    int64
	tradesSinceC int64

	// Lookahead buffer for structural coalescing before Flush
	pending []episode
}

/*
NewDetector creates the detector for symbol's tape in epoch. Every excursion
class is defined against round-trip friction, so a price system with
reference cash and a taker fee for symbol are required.
*/
func NewDetector(
	ctx context.Context,
	price *broker.Price,
	epoch uint64,
	symbol string,
) (*Detector, error) {
	if price == nil || price.ReferenceCash() == nil {
		return nil, errnie.Err(errnie.Validation, "[detector] price system with reference cash required", nil)
	}

	feeRate, ok := price.FeeRate(symbol)

	if !ok || feeRate <= 0 || feeRate >= 1 {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[detector] %s needs a taker fee in (0, 1), have %v (known: %v)", symbol, feeRate, ok,
		), nil)
	}

	return &Detector{
		System:                 runtime.NewSystem(ctx, "detector"),
		price:                  price,
		symbol:                 symbol,
		epoch:                  epoch,
		roundTrip:              (1.0+feeRate)/(1.0-feeRate) - 1.0,
		retraceRatio:           0.32, // Pullback must retrace >= 32% of |C - B| (and clear friction) to end a leg
		maxContinuationRetrace: 0.58, // Higher-low dips < 58% merge into one macro leg when C is broken
		boxOverlapRatio:        0.68, // Alternating swings overlapping >= 68% coalesce into "chop"
		minFlatTrades:          24,   // Minimum trades within flatBand to emit an independent "flat" episode
		maxFlatGlitchTrades:    2,    // Ignore 1-2 trade stray wicks inside a long flat stretch
		minEpisodeTrades:       8,    // Absorb micro-fragments shorter than 8 trades into adjacent episodes
		minStagnationTrades:    80,   // Minimum trades after C before checking horizontal time-exhaustion
		stagnationMultiplier:   3,    // Exhaust leg into chop/flat if sideways trades after C >= 3x impulse trades
		state:                  stateInit,
		pending:                make([]episode, 0, 8),
	}, nil
}

/*
Step feeds one trade of the detector's symbol from stored tick number tick,
with the touch that tick recorded for the symbol, and returns the excursions
this trade confirmed. A trade of another symbol, a non-positive price, or a
friction computation that fails is an error.
*/
func (detector *Detector) Step(
	tick uint64, trade kraken.TradeData, touch broker.PriceLayer,
) ([]*data.Value, error) {
	if trade.Symbol != detector.symbol {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[detector] %s detector fed a %s trade", detector.symbol, trade.Symbol,
		), nil)
	}

	if trade.Price.Sign() <= 0 {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[detector] %s trade %d at tick %d has non-positive price %s",
			trade.Symbol, trade.TradeID, tick, trade.Price.String(),
		), nil)
	}

	price := trade.Price
	detector.seq++

	pt := tradePoint{
		tick:   int64(tick),
		seqIdx: detector.seq,
		at:     trade.Timestamp.UnixNano(),
		price:  &price,
		priceF: price.Float64(),
		bid:    touch.Bid,
		ask:    touch.Ask,
	}

	detector.updateMicrostructure(pt)

	switch detector.state {
	case stateInit:
		detector.initFromPoint(pt)
	case stateUndecided:
		detector.stepUndecided(pt)
	case stateUp:
		detector.stepUp(pt)
	case stateDown:
		detector.stepDown(pt)
	}

	return detector.take()
}

/*
Close flushes the episodes still held for coalescing at the end of the tape.
*/
func (detector *Detector) Close() ([]*data.Value, error) {
	for len(detector.pending) > 0 {
		oldest := detector.pending[0]
		detector.pending = detector.pending[1:]
		detector.flush(oldest)
	}

	return detector.take()
}

func (detector *Detector) take() ([]*data.Value, error) {
	results := detector.results
	detector.results = nil

	if detector.failure != nil {
		return nil, detector.failure
	}

	return results, nil
}

/*
updateMicrostructure tracks single-trade direction-flip bounces to measure the
effective bid-ask bounce floor of the book (e.g. ALTHEA 10% spread, AMI 36% spread).
*/
func (detector *Detector) updateMicrostructure(pt tradePoint) {
	if detector.prevTrade != nil && detector.prevTrade.priceF > 0 {
		delta := (pt.priceF - detector.prevTrade.priceF) / detector.prevTrade.priceF
		absDelta := math.Abs(delta)

		if absDelta > 0 {
			if detector.prevDelta != 0 && (delta*detector.prevDelta) < 0 {
				capped := math.Min(absDelta, 0.25)
				if detector.emaBounce == 0 {
					detector.emaBounce = capped
				} else {
					detector.emaBounce = 0.92*detector.emaBounce + 0.08*capped
				}
			} else {
				detector.emaBounce *= 0.992
			}
			detector.prevDelta = delta
		}
	}

	cp := pt
	detector.prevTrade = &cp
}

/*
effectiveDeadband combines the venue round-trip taker fee with the rolling
bid-ask bounce floor so spread bounce never triggers false directional legs.
*/
func (detector *Detector) effectiveDeadband() float64 {
	feeFloor := 2.0 * detector.roundTrip
	spreadFloor := 2.2 * detector.emaBounce

	return math.Max(feeFloor, spreadFloor)
}

/*
clearsEconomicFriction verifies that a move from lowPrice to highPrice clears
both the dynamic microstructure deadband and broker.Price's RoundTrip fee math.
A RoundTrip that cannot be computed fails the detector: no class is assigned
on a guess.
*/
func (detector *Detector) clearsEconomicFriction(lowPrice, highPrice *decimal.Decimal) bool {
	if lowPrice == nil || highPrice == nil || lowPrice.Sign() <= 0 || highPrice.Cmp(lowPrice) <= 0 {
		return false
	}

	rel := (highPrice.Float64() - lowPrice.Float64()) / lowPrice.Float64()
	if rel < detector.effectiveDeadband() {
		return false
	}

	pnl, _, err := detector.price.RoundTrip(detector.symbol, lowPrice, highPrice)

	if err != nil {
		if detector.failure == nil {
			detector.failure = errnie.Err(errnie.Validation, fmt.Sprintf(
				"[detector] %s round trip %s -> %s", detector.symbol, lowPrice.String(), highPrice.String(),
			), err)
		}

		return false
	}

	return pnl.Sign() > 0
}

func (detector *Detector) flatBand() float64 {
	return 0.25 * detector.roundTrip
}

func (detector *Detector) initFromPoint(pt tradePoint) {
	detector.segStart = pt
	detector.runLow = pt
	detector.runHigh = pt
	detector.resetFlatTracker(pt)
	detector.state = stateUndecided
}

func (detector *Detector) resetFlatTracker(pt tradePoint) {
	detector.flatStart = pt
	detector.flatLast = pt
	detector.flatMin = pt.priceF
	detector.flatMax = pt.priceF
	detector.flatCount = 1
	detector.flatGlitchCnt = 0
}

func (detector *Detector) stepUndecided(pt tradePoint) {
	fBand := detector.flatBand()

	// 1. Track sustained flat regimes (subordinated until minFlatTrades is reached).
	candMin := math.Min(detector.flatMin, pt.priceF)
	candMax := math.Max(detector.flatMax, pt.priceF)
	inFlat := candMin > 0 && ((candMax-candMin)/candMin) <= fBand

	if inFlat {
		detector.flatMin = candMin
		detector.flatMax = candMax
		detector.flatLast = pt
		detector.flatCount++
		detector.flatGlitchCnt = 0
	} else {
		detector.flatGlitchCnt++

		if detector.flatGlitchCnt > detector.maxFlatGlitchTrades {
			if detector.flatCount >= detector.minFlatTrades &&
				detector.flatLast.seqIdx > detector.flatStart.seqIdx {

				if detector.flatStart.seqIdx > detector.segStart.seqIdx {
					detector.enqueueEpisode(episode{
						class:  "chop",
						start:  detector.segStart,
						end:    detector.flatStart,
						lowP:   math.Min(detector.segStart.priceF, detector.flatStart.priceF),
						highP:  math.Max(detector.segStart.priceF, detector.flatStart.priceF),
						trades: detector.flatStart.seqIdx - detector.segStart.seqIdx,
					})
				}

				detector.enqueueEpisode(episode{
					class:  "flat",
					start:  detector.flatStart,
					end:    detector.flatLast,
					lowP:   detector.flatMin,
					highP:  detector.flatMax,
					trades: detector.flatCount,
				})

				detector.segStart = detector.flatLast
				detector.runLow = detector.flatLast
				detector.runHigh = detector.flatLast
			}

			detector.resetFlatTracker(pt)
		}
	}

	// 2. Track running extremes of the undecided window.
	if pt.price.Cmp(detector.runLow.price) <= 0 {
		detector.runLow = pt
	}

	if pt.price.Cmp(detector.runHigh.price) >= 0 {
		detector.runHigh = pt
	}

	// 3. Check if a breakout from runLow or runHigh clears economic friction.
	upClears := pt.seqIdx > detector.runLow.seqIdx &&
		detector.clearsEconomicFriction(detector.runLow.price, pt.price)

	downClears := pt.seqIdx > detector.runHigh.seqIdx &&
		detector.clearsEconomicFriction(pt.price, detector.runHigh.price)

	if upClears {
		if detector.runLow.seqIdx > detector.segStart.seqIdx {
			detector.enqueueEpisode(episode{
				class:  "chop",
				start:  detector.segStart,
				end:    detector.runLow,
				lowP:   detector.runLow.priceF,
				highP:  detector.runHigh.priceF,
				trades: detector.runLow.seqIdx - detector.segStart.seqIdx,
			})
		}

		detector.enterUp(detector.runLow, pt)
		return
	}

	if downClears {
		if detector.runHigh.seqIdx > detector.segStart.seqIdx {
			detector.enqueueEpisode(episode{
				class:  "chop",
				start:  detector.segStart,
				end:    detector.runHigh,
				lowP:   detector.runLow.priceF,
				highP:  detector.runHigh.priceF,
				trades: detector.runHigh.seqIdx - detector.segStart.seqIdx,
			})
		}

		detector.enterDown(detector.runHigh, pt)
	}
}

func (detector *Detector) enterUp(b, c tradePoint) {
	detector.state = stateUp
	detector.legB = b
	detector.legC = c
	detector.legExtreme = c
	detector.legTrades = c.seqIdx - b.seqIdx + 1
	detector.tradesSinceC = 0
	detector.tryAdoptPriorUp()
}

func (detector *Detector) enterDown(b, c tradePoint) {
	detector.state = stateDown
	detector.legB = b
	detector.legC = c
	detector.legExtreme = c
	detector.legTrades = c.seqIdx - b.seqIdx + 1
	detector.tradesSinceC = 0
	detector.tryAdoptPriorDown()
}

func (detector *Detector) stepUp(pt tradePoint) {
	detector.legTrades++

	if pt.price.Cmp(detector.legC.price) >= 0 {
		detector.legC = pt
		detector.legExtreme = pt
		detector.tradesSinceC = 0
		detector.tryAdoptPriorUp()
		return
	}

	detector.tradesSinceC++

	if pt.price.Cmp(detector.legExtreme.price) < 0 {
		detector.legExtreme = pt
	}

	legRange := detector.legC.priceF - detector.legB.priceF
	retraceAbs := detector.legC.priceF - pt.priceF

	var retraceOfLeg float64

	if legRange > 0 {
		retraceOfLeg = retraceAbs / legRange
	}

	brokeOrigin := pt.price.Cmp(detector.legB.price) < 0
	retraceClearsFee := detector.clearsEconomicFriction(pt.price, detector.legC.price)
	clearedProportional := retraceClearsFee && retraceOfLeg >= detector.retraceRatio

	if brokeOrigin || clearedProportional {
		detector.completeUpLeg()

		if retraceClearsFee && pt.seqIdx > detector.legC.seqIdx {
			detector.enterDown(detector.legC, pt)
		} else {
			detector.initFromPoint(detector.legC)
			detector.stepUndecided(pt)
		}

		return
	}

	// Horizontal time-exhaustion: if price stalls below C for >= 3x the impulse duration,
	// end the up leg at C and transition the post-C sideways box into chop/flat.
	impulseTrades := max(1, detector.legC.seqIdx-detector.legB.seqIdx)
	stagnationLimit := max(detector.minStagnationTrades, impulseTrades*detector.stagnationMultiplier)

	if detector.tradesSinceC >= stagnationLimit {
		detector.completeUpLeg()
		detector.initFromPoint(detector.legC)
		detector.stepUndecided(pt)
	}
}

func (detector *Detector) stepDown(pt tradePoint) {
	detector.legTrades++

	if pt.price.Cmp(detector.legC.price) <= 0 {
		detector.legC = pt
		detector.legExtreme = pt
		detector.tradesSinceC = 0
		detector.tryAdoptPriorDown()
		return
	}

	detector.tradesSinceC++

	if pt.price.Cmp(detector.legExtreme.price) > 0 {
		detector.legExtreme = pt
	}

	legRange := detector.legB.priceF - detector.legC.priceF
	bounceAbs := pt.priceF - detector.legC.priceF

	var retraceOfLeg float64

	if legRange > 0 {
		retraceOfLeg = bounceAbs / legRange
	}

	brokeOrigin := pt.price.Cmp(detector.legB.price) > 0
	bounceClearsFee := detector.clearsEconomicFriction(detector.legC.price, pt.price)
	clearedProportional := bounceClearsFee && retraceOfLeg >= detector.retraceRatio

	if brokeOrigin || clearedProportional {
		detector.completeDownLeg()

		if bounceClearsFee && pt.seqIdx > detector.legC.seqIdx {
			detector.enterUp(detector.legC, pt)
		} else {
			detector.initFromPoint(detector.legC)
			detector.stepUndecided(pt)
		}

		return
	}

	impulseTrades := max(1, detector.legC.seqIdx-detector.legB.seqIdx)
	stagnationLimit := max(detector.minStagnationTrades, impulseTrades*detector.stagnationMultiplier)

	if detector.tradesSinceC >= stagnationLimit {
		detector.completeDownLeg()
		detector.initFromPoint(detector.legC)
		detector.stepUndecided(pt)
	}
}

func (detector *Detector) completeUpLeg() {
	trades := detector.legC.seqIdx - detector.legB.seqIdx
	class := "up"

	if trades < 1 || !detector.clearsEconomicFriction(detector.legB.price, detector.legC.price) {
		class = "chop"
	}

	detector.enqueueEpisode(episode{
		class:  class,
		start:  detector.legB,
		end:    detector.legC,
		lowP:   detector.legB.priceF,
		highP:  detector.legC.priceF,
		trades: trades,
	})
}

func (detector *Detector) completeDownLeg() {
	trades := detector.legC.seqIdx - detector.legB.seqIdx
	class := "down"

	if trades < 1 || !detector.clearsEconomicFriction(detector.legC.price, detector.legB.price) {
		class = "chop"
	}

	detector.enqueueEpisode(episode{
		class:  class,
		start:  detector.legB,
		end:    detector.legC,
		lowP:   detector.legC.priceF,
		highP:  detector.legB.priceF,
		trades: trades,
	})
}

/*
tryAdoptPriorUp merges a staircase uptrend (up1 -> shallow dip -> up2) into one
macro up leg as soon as the active leg breaks above up1's peak C1, provided the
intervening pullback held a higher low above B1 and did not stagnate into a
long sideways regime.
*/
func (detector *Detector) tryAdoptPriorUp() {
	n := len(detector.pending)

	if n == 0 {
		return
	}

	// Case 1: pending ends directly with an "up" episode.
	if detector.pending[n-1].class == "up" {
		prior := detector.pending[n-1]
		if detector.legC.price.Cmp(prior.end.price) >= 0 &&
			detector.legB.price.Cmp(prior.start.price) >= 0 {
			detector.legB = prior.start
			detector.pending = detector.pending[:n-1]
		}
		return
	}

	// Case 2: pending ends with [..., priorUp, dip] where dip is a shallow pullback.
	if n >= 2 && detector.pending[n-2].class == "up" {
		priorUp := detector.pending[n-2]
		dip := detector.pending[n-1]

		priorRange := priorUp.end.priceF - priorUp.start.priceF
		dipDepth := priorUp.end.priceF - detector.legB.priceF

		if priorRange > 0 &&
			detector.legC.price.Cmp(priorUp.end.price) >= 0 &&
			detector.legB.price.Cmp(priorUp.start.price) > 0 &&
			(dipDepth/priorRange) <= detector.maxContinuationRetrace &&
			dip.trades <= max(detector.minStagnationTrades, priorUp.trades*2) {
			detector.legB = priorUp.start
			detector.pending = detector.pending[:n-2]
		}
	}
}

/*
tryAdoptPriorDown mirrors tryAdoptPriorUp for staircase downtrends.
*/
func (detector *Detector) tryAdoptPriorDown() {
	n := len(detector.pending)

	if n == 0 {
		return
	}

	if detector.pending[n-1].class == "down" {
		prior := detector.pending[n-1]

		if detector.legC.price.Cmp(prior.end.price) <= 0 &&
			detector.legB.price.Cmp(prior.start.price) <= 0 {
			detector.legB = prior.start
			detector.pending = detector.pending[:n-1]
		}

		return
	}

	if n >= 2 && detector.pending[n-2].class == "down" {
		priorDown := detector.pending[n-2]
		bounce := detector.pending[n-1]

		priorRange := priorDown.start.priceF - priorDown.end.priceF
		bounceHeight := detector.legB.priceF - priorDown.end.priceF

		if priorRange > 0 &&
			detector.legC.price.Cmp(priorDown.end.price) <= 0 &&
			detector.legB.price.Cmp(priorDown.start.price) < 0 &&
			(bounceHeight/priorRange) <= detector.maxContinuationRetrace &&
			bounce.trades <= max(detector.minStagnationTrades, priorDown.trades*2) {
			detector.legB = priorDown.start
			detector.pending = detector.pending[:n-2]
		}
	}
}

/*
enqueueEpisode coalesces adjacent identical regimes, absorbs micro-slivers, and
demotes self-canceling box whip-saws into a single contiguous "chop" stretch.
*/
func (detector *Detector) enqueueEpisode(ep episode) {
	if ep.end.seqIdx <= ep.start.seqIdx {
		return
	}

	// 1. Absorb micro-slivers into the preceding episode if one exists.
	if ep.trades < detector.minEpisodeTrades && len(detector.pending) > 0 {
		last := &detector.pending[len(detector.pending)-1]
		last.end = ep.end
		last.lowP = math.Min(last.lowP, ep.lowP)
		last.highP = math.Max(last.highP, ep.highP)
		last.trades = last.end.seqIdx - last.start.seqIdx
		return
	}

	// 2. Merge adjacent episodes of the same class.
	if len(detector.pending) > 0 && detector.pending[len(detector.pending)-1].class == ep.class {
		last := &detector.pending[len(detector.pending)-1]
		last.end = ep.end
		last.lowP = math.Min(last.lowP, ep.lowP)
		last.highP = math.Max(last.highP, ep.highP)
		last.trades = last.end.seqIdx - last.start.seqIdx
		return
	}

	detector.pending = append(detector.pending, ep)

	// 3. Detect 3-swing overlapping whip-saws inside a range box and collapse into "chop".
	detector.coalesceBoxWhipsaws()

	// 4. Flush the oldest episode once it is safely beyond the lookahead window.
	for len(detector.pending) > 3 {
		oldest := detector.pending[0]
		detector.pending = detector.pending[1:]
		detector.flush(oldest)
	}
}

/*
coalesceBoxWhipsaws detects alternating up/down swings that retrace >= 68% of
each other without escaping the bounding box and collapses them into "chop".
*/
func (detector *Detector) coalesceBoxWhipsaws() {
	n := len(detector.pending)

	if n < 3 {
		return
	}

	s1 := detector.pending[n-3]
	s2 := detector.pending[n-2]
	s3 := detector.pending[n-1]

	isAlternating := (s1.class == "up" && s2.class == "down" && s3.class == "up") ||
		(s1.class == "down" && s2.class == "up" && s3.class == "down")

	if !isAlternating {
		return
	}

	amp1 := math.Abs(s1.end.priceF - s1.start.priceF)
	amp2 := math.Abs(s2.end.priceF - s2.start.priceF)
	amp3 := math.Abs(s3.end.priceF - s3.start.priceF)

	if amp1 <= 0 || amp2 <= 0 {
		return
	}

	r12 := amp2 / amp1
	r23 := amp3 / amp2

	// If both counter-swings wipe out most of the prior swing without breaking out (> 1.35x),
	// they are oscillating inside a horizontal chop box.
	if r12 >= detector.boxOverlapRatio && r12 <= 1.35 &&
		r23 >= detector.boxOverlapRatio && r23 <= 1.35 {
		merged := episode{
			class:  "chop",
			start:  s1.start,
			end:    s3.end,
			lowP:   math.Min(s1.lowP, math.Min(s2.lowP, s3.lowP)),
			highP:  math.Max(s1.highP, math.Max(s2.highP, s3.highP)),
			trades: s3.end.seqIdx - s1.start.seqIdx,
		}
		detector.pending = detector.pending[:n-3]
		detector.enqueueEpisode(merged)
	}
}

/*
flush emits one confirmed episode as an excursion Value keyed
data.Prefix("detector", class, symbol, epoch) at C's tick. Its long-side
economics need the touch at B (ask) and C (bid): an episode the tape cannot
price is a detector failure, not a zero.
*/
func (detector *Detector) flush(ep episode) {
	b, c := ep.start, ep.end

	if b.ask == nil || c.bid == nil {
		if detector.failure == nil {
			detector.failure = errnie.Err(errnie.NotFound, fmt.Sprintf(
				"[detector] %s %s excursion ticks %d..%d: tick recorded no ask at B or no bid at C",
				detector.symbol, ep.class, b.tick, c.tick,
			), nil)
		}

		return
	}

	halfMoveSeqIdx := c.seqIdx - b.seqIdx
	halfMoveTick := c.tick - b.tick

	value := &data.Value{
		Origin:    detectorOrigin,
		Role:      ep.class,
		Scope:     detector.symbol,
		Epoch:     detector.epoch,
		Tick:      uint64(c.tick),
		SeqIdx:    uint64(c.seqIdx),
		Timestamp: uint64(c.at),
	}
	value.FormatPrefix()

	data.AppendVector(value, []float64{
		float64(b.seqIdx - halfMoveSeqIdx),
		float64(b.tick - halfMoveTick),
		float64(b.seqIdx),
		float64(b.tick),
		float64(c.seqIdx),
		float64(c.tick),
		float64(c.seqIdx + halfMoveSeqIdx),
		float64(c.tick + halfMoveTick),
		b.priceF,
		c.priceF,
		b.ask.Float64(),
		c.bid.Float64(),
		float64(b.at),
		float64(c.at),
	}, ExcursionKeys...)

	detector.results = append(detector.results, value)
}
