package broker

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

const (
	tapeRhythmCap   = 32
	tapeFlipTolFrac = 0.01
	tapeEwmaAlpha   = 0.15
)

/*
symbolTape is per-symbol so tape uptake on BTC does not serialize on ETH prints.
Buy and sell minute buckets share the same clock so net pressure and rush
splits stay aligned with the board's recent/prior window.
*/
type symbolTape struct {
	mu sync.Mutex

	buyBuckets      []float64
	sellBuckets     []float64
	rushBuyBuckets  []float64
	rushSellBuckets []float64
	flipBuckets     []float64
	bucketMinute    []int64

	lastSide  market.Side
	lastQuote float64

	ewma         float64
	ewmaVar      float64
	ewmaInit     bool
	lastEwmaMin  int64
	peakNotional float64

	lastAt  time.Time
	gaps    [tapeRhythmCap]float64
	sizes   [tapeRhythmCap]float64
	gapN    int
	sizeN   int
	gapIdx  int
	sizeIdx int
}

/*
Tape keeps per-symbol 1-minute quote-notional buckets so entry can compare
the last risk.tape_window with the equal window before it — buy, sell, market
rush, flip churn, and a longer-horizon EWMA baseline for double-conditioning.
*/
type Tape struct {
	symbols sync.Map // map[string]*symbolTape
}

/*
TapeReading is one symbol's conditioned tape state for gates and board scoring.
*/
type TapeReading struct {
	BuyRecent, BuyPrior   float64
	SellRecent, SellPrior float64
	RushBuy, RushSell     float64
	NetFraction           float64
	FlipChurn             float64
	Ratio                 float64
	ConditionedRatio      float64
	Baseline              float64
	Rhythm                float64 // 0 mechanical → 1 organic
}

/*
NewTape is lazy per symbol — most of the universe never prints during a session.
*/
func NewTape() *Tape {
	return &Tape{}
}

func newSymbolTape() *symbolTape {
	n := 2 * int(system.Cfg.Risk.TapeWindow/time.Minute)

	if n < 2 {
		n = 2
	}

	return &symbolTape{
		buyBuckets:      make([]float64, n),
		sellBuckets:     make([]float64, n),
		rushBuyBuckets:  make([]float64, n),
		rushSellBuckets: make([]float64, n),
		flipBuckets:     make([]float64, n),
		bucketMinute:    make([]int64, n),
	}
}

func (tape *Tape) lane(symbol string) *symbolTape {
	if raw, ok := tape.symbols.Load(symbol); ok {
		return raw.(*symbolTape)
	}

	raw, _ := tape.symbols.LoadOrStore(symbol, newSymbolTape())
	return raw.(*symbolTape)
}

/*
Note adds buy quote dollars to the print's minute bucket — kept for tests and
callers that only track uptake without side/order-type context.
*/
func (tape *Tape) Note(symbol string, quote float64, at time.Time) {
	tape.Observe(symbol, market.SideBuy, "", quote, at)
}

/*
Observe records one print's quote notional by side and order type, updates
flip churn, EWMA baseline, and inter-print rhythm samples.
*/
func (tape *Tape) Observe(
	symbol string,
	side market.Side,
	orderType market.OrderType,
	quote float64,
	at time.Time,
) {
	if symbol == "" || quote <= 0 || at.IsZero() {
		return
	}

	lane := tape.lane(symbol)
	lane.mu.Lock()
	defer lane.mu.Unlock()

	n := int64(len(lane.buyBuckets))
	minute := at.Unix() / 60
	idx := minute % n

	if lane.bucketMinute[idx] != minute {
		lane.rollEwmaLocked(minute)
		lane.bucketMinute[idx] = minute
		lane.buyBuckets[idx] = 0
		lane.sellBuckets[idx] = 0
		lane.rushBuyBuckets[idx] = 0
		lane.rushSellBuckets[idx] = 0
		lane.flipBuckets[idx] = 0
	}

	switch side {
	case market.SideBuy:
		lane.buyBuckets[idx] += quote
		if orderType == market.OrderTypeMarket {
			lane.rushBuyBuckets[idx] += quote
		}
	case market.SideSell:
		lane.sellBuckets[idx] += quote
		if orderType == market.OrderTypeMarket {
			lane.rushSellBuckets[idx] += quote
		}
	}

	if lane.lastQuote > 0 && lane.lastSide != "" && side != lane.lastSide {
		den := math.Max(lane.lastQuote, quote)
		if den > 0 && math.Abs(lane.lastQuote-quote)/den <= tapeFlipTolFrac {
			lane.flipBuckets[idx]++
		}
	}

	lane.lastSide = side
	lane.lastQuote = quote
	lane.noteRhythmLocked(quote, at)
}

func (lane *symbolTape) rollEwmaLocked(minute int64) {
	if lane.lastEwmaMin == 0 {
		lane.lastEwmaMin = minute
		return
	}

	if minute <= lane.lastEwmaMin {
		return
	}

	window := int64(system.Cfg.Risk.TapeWindow / time.Minute)
	if window < 1 {
		window = 1
	}

	for m := lane.lastEwmaMin + 1; m <= minute; m++ {
		sum := 0.0
		for i := int64(0); i < window; i++ {
			past := m - 1 - i
			idx := past % int64(len(lane.buyBuckets))
			if lane.bucketMinute[idx] == past {
				sum += lane.buyBuckets[idx]
			}
		}

		if !lane.ewmaInit {
			lane.ewma = sum
			lane.ewmaVar = 0
			lane.ewmaInit = true
		} else {
			delta := sum - lane.ewma
			lane.ewma += tapeEwmaAlpha * delta
			lane.ewmaVar = (1 - tapeEwmaAlpha) *
				(lane.ewmaVar + tapeEwmaAlpha*delta*delta)
		}

		if sum > lane.peakNotional {
			lane.peakNotional = sum
		}
	}

	lane.lastEwmaMin = minute
}

func (lane *symbolTape) noteRhythmLocked(quote float64, at time.Time) {
	if !lane.lastAt.IsZero() {
		gap := at.Sub(lane.lastAt).Seconds()
		if gap > 0 {
			lane.gaps[lane.gapIdx%tapeRhythmCap] = gap
			lane.gapIdx++
			if lane.gapN < tapeRhythmCap {
				lane.gapN++
			}
		}
	}

	lane.sizes[lane.sizeIdx%tapeRhythmCap] = quote
	lane.sizeIdx++
	if lane.sizeN < tapeRhythmCap {
		lane.sizeN++
	}

	lane.lastAt = at
}

/*
Uptake is buy-tape recent vs prior — BookIntact and legacy callers compare
rising buy flow against the equal prior window.
*/
func (tape *Tape) Uptake(symbol string, now time.Time) (recent, prior float64) {
	reading := tape.Reading(symbol, now, 0)
	return reading.BuyRecent, reading.BuyPrior
}

/*
Reading returns conditioned buy/sell/rush/net/flip/rhythm state for one symbol.
dayVolumeUSD floors the prior window so dormant wake-ups cannot explode ratios.
*/
func (tape *Tape) Reading(symbol string, now time.Time, dayVolumeUSD float64) TapeReading {
	raw, ok := tape.symbols.Load(symbol)
	if !ok {
		return TapeReading{}
	}

	lane := raw.(*symbolTape)
	lane.mu.Lock()
	defer lane.mu.Unlock()

	n := int64(len(lane.buyBuckets))
	window := int64(system.Cfg.Risk.TapeWindow / time.Minute)
	if window < 1 {
		window = 1
	}
	current := now.Unix() / 60
	lane.rollEwmaLocked(current)

	var reading TapeReading
	var flips float64

	for i := int64(0); i < window; i++ {
		recentMinute := current - i
		idx := recentMinute % n
		if lane.bucketMinute[idx] == recentMinute {
			reading.BuyRecent += lane.buyBuckets[idx]
			reading.SellRecent += lane.sellBuckets[idx]
			reading.RushBuy += lane.rushBuyBuckets[idx]
			reading.RushSell += lane.rushSellBuckets[idx]
			flips += lane.flipBuckets[idx]
		}

		priorMinute := current - window - i
		priorIdx := priorMinute % n
		if lane.bucketMinute[priorIdx] == priorMinute {
			reading.BuyPrior += lane.buyBuckets[priorIdx]
			reading.SellPrior += lane.sellBuckets[priorIdx]
		}
	}

	gross := reading.BuyRecent + reading.SellRecent
	if gross > 0 {
		reading.NetFraction = (reading.BuyRecent - reading.SellRecent) / gross
	}

	net := math.Abs(reading.BuyRecent - reading.SellRecent)
	reading.FlipChurn = flips / (net + 1)

	if reading.BuyPrior > 0 {
		reading.Ratio = reading.BuyRecent / reading.BuyPrior
	}

	reading.Baseline = lane.ewma
	reading.ConditionedRatio = conditionedTapeRatio(
		reading.BuyRecent,
		reading.BuyPrior,
		lane.ewma,
		math.Sqrt(math.Max(lane.ewmaVar, 0)),
		lane.peakNotional,
		dayVolumeUSD,
		window,
	)
	reading.Rhythm = rhythmScore(lane.gaps[:lane.gapN], lane.sizes[:lane.sizeN])

	return reading
}

func conditionedTapeRatio(
	recent, prior, ewma, ewmaStd, peak, dayVolumeUSD float64,
	windowMinutes int64,
) float64 {
	risk := system.Cfg.Risk
	// Floor prior with the absolute notional gate so $1→$251 cannot explode
	// past a real multi-thousand surge. Day-average and EWMA tighten further
	// only as soft blend terms — they must not seal healthy accelerations.
	floor := prior
	if risk.MinTapeNotionalUSD > floor {
		floor = risk.MinTapeNotionalUSD
	}

	if floor <= 0 {
		if recent <= 0 {
			return 0
		}
		if risk.MinTapeNotionalUSD > 0 {
			return recent / risk.MinTapeNotionalUSD
		}
		return 0
	}

	ratio := recent / floor

	if ewma > 0 {
		zFloor := ewma + ewmaStd
		if risk.MinTapeNotionalUSD > zFloor {
			zFloor = risk.MinTapeNotionalUSD
		}
		z := (recent - ewma) / zFloor
		if z > 0 {
			ratio = math.Min(ratio, math.Max(1, 1+z))
		}
	}

	if peak > risk.MinTapeNotionalUSD && recent < 0.60*peak {
		ratio = math.Min(ratio, recent/(0.60*peak))
	}

	if dayVolumeUSD > 0 && windowMinutes > 0 {
		periods := (24.0 * 60.0) / float64(windowMinutes)
		if periods > 0 {
			dayAvg := dayVolumeUSD / periods
			if dayAvg > risk.MinTapeNotionalUSD && recent < 0.25*dayAvg {
				ratio = math.Min(ratio, recent/(0.25*dayAvg))
			}
		}
	}

	return ratio
}

func rhythmScore(gaps, sizes []float64) float64 {
	gapCV := sampleCV(gaps)
	sizeCV := sampleCV(sizes)

	if gapCV < 0 && sizeCV < 0 {
		return 1
	}

	score := 0.0
	n := 0
	if gapCV >= 0 {
		score += math.Min(gapCV, 1)
		n++
	}
	if sizeCV >= 0 {
		score += math.Min(sizeCV, 1)
		n++
	}
	if n == 0 {
		return 1
	}
	return score / float64(n)
}

func sampleCV(samples []float64) float64 {
	if len(samples) < 4 {
		return -1
	}

	mean := 0.0
	for _, v := range samples {
		mean += v
	}
	mean /= float64(len(samples))
	if mean <= 0 {
		return -1
	}

	var sumSq float64
	for _, v := range samples {
		d := v - mean
		sumSq += d * d
	}

	return math.Sqrt(sumSq/float64(len(samples))) / mean
}
