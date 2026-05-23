package trader

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type tradeCandidate struct {
	pair       asset.Pair
	symbol     string
	confidence float64
	support    int
	regime     string
	reason     string
	measType   engine.MeasurementType
}

func (crypto *Crypto) rankCandidates(batch []engine.Measurement) []tradeCandidate {
	bySymbol := make(map[string]tradeCandidate)

	for _, measurement := range batch {
		if measurement.Err != nil || measurement.Confidence <= 0 || len(measurement.Pairs) == 0 {
			continue
		}

		if !tradeableType(measurement.Type) {
			continue
		}

		pair := measurement.Pairs[0]
		symbol := pairSymbol(pair)

		if symbol == "" {
			continue
		}

		regime := measurement.Regime
		if regime == "" {
			regime = regimeForType(measurement.Type)
		}

		reason := measurement.Reason
		if reason == "" {
			reason = "ok"
		}

		candidate := bySymbol[symbol]
		next := tradeCandidate{
			pair:       pair,
			symbol:     symbol,
			confidence: measurement.Confidence,
			support:    1,
			regime:     regime,
			reason:     reason,
			measType:   measurement.Type,
		}

		if candidate.symbol != "" {
			next.confidence = candidate.confidence + measurement.Confidence
			next.support = candidate.support + 1

			if !preferCandidate(next, candidate) {
				next.regime = candidate.regime
				next.reason = candidate.reason
				next.measType = candidate.measType
			}
		}

		next.confidence = crypto.boostConfidence(symbol, next.confidence)
		bySymbol[symbol] = next
	}

	if len(bySymbol) == 0 {
		return nil
	}

	ranked := make([]tradeCandidate, 0, len(bySymbol))

	for _, candidate := range bySymbol {
		ranked = append(ranked, candidate)
	}

	sortCandidates(ranked)

	return ranked
}

func tradeableType(measType engine.MeasurementType) bool {
	switch measType {
	case engine.Pump, engine.Momentum, engine.Flow, engine.Causal:
		return true
	default:
		return false
	}
}

func regimeForType(measType engine.MeasurementType) string {
	switch measType {
	case engine.Pump:
		return "pump"
	case engine.Momentum:
		return "momentum"
	case engine.Flow:
		return "flow"
	case engine.Causal:
		return "causal"
	default:
		return "unknown"
	}
}

func preferCandidate(next, current tradeCandidate) bool {
	if current.symbol == "" {
		return true
	}

	nextPriority := typePriority(next.measType)
	currentPriority := typePriority(current.measType)

	if nextPriority != currentPriority {
		return nextPriority > currentPriority
	}

	return next.confidence > current.confidence
}

func typePriority(measType engine.MeasurementType) int {
	switch measType {
	case engine.Pump:
		return 3
	case engine.Flow:
		return 2
	case engine.Causal:
		return 2
	case engine.Momentum:
		return 1
	default:
		return 0
	}
}

func (crypto *Crypto) canEnter(candidate tradeCandidate) bool {
	if len(crypto.holds) >= config.System.MaxSlots {
		return false
	}

	pumpSlots, scalpSlots := crypto.regimeSlotUse()

	switch candidate.regime {
	case "pump":
		maxPump := maxPumpSlots()
		if pumpSlots >= maxPump && len(crypto.holds) >= maxPump {
			return false
		}
	case "momentum":
		maxScalp := maxScalpSlots()
		if scalpSlots >= maxScalp {
			return false
		}
	}

	return true
}

func maxPumpSlots() int {
	if config.System.MaxSlots <= 1 {
		return 1
	}

	return (config.System.MaxSlots + 1) / 2
}

func maxScalpSlots() int {
	return config.System.MaxSlots
}

func (crypto *Crypto) regimeSlotUse() (pumpSlots, scalpSlots int) {
	for _, hold := range crypto.holds {
		switch hold.regime {
		case "pump":
			pumpSlots++
		case "momentum", "flow", "causal":
			scalpSlots++
		}
	}

	return pumpSlots, scalpSlots
}

func (crypto *Crypto) minHoldForRegime(regime string) time.Duration {
	switch regime {
	case "momentum":
		return config.System.ScalpHoldBeforeExit
	case "flow", "causal":
		return config.System.FlowHoldBeforeExit
	default:
		return config.System.MinHoldBeforeRotate
	}
}

func (crypto *Crypto) entryFill(symbol string) (float64, float64, bool) {
	last, bid, ask, _, ok := crypto.quote(symbol)
	if !ok || last <= 0 {
		return 0, 0, false
	}

	fill := config.System.SlippagePrice(last, bid, ask, "buy", config.System.SlippageBPS)

	return fill, trailPct(last, bid, ask), true
}

func (crypto *Crypto) exitFill(symbol string) (float64, bool) {
	last, bid, ask, _, ok := crypto.quote(symbol)
	if !ok || last <= 0 {
		return 0, false
	}

	fill := config.System.SlippagePrice(last, bid, ask, "sell", config.System.SlippageBPS)

	return fill, true
}

func (crypto *Crypto) quote(symbol string) (last, bid, ask, changePct float64, ok bool) {
	if crypto.prices == nil {
		return 0, 0, 0, 0, false
	}

	quoteReader, typed := crypto.prices.(QuoteReader)
	if typed {
		return quoteReader.Quote(symbol)
	}

	last, ok = crypto.prices.Last(symbol)

	return last, 0, 0, 0, ok
}

func trailPct(last, bid, ask float64) float64 {
	if last <= 0 {
		return 0
	}

	spread := 0.0

	if bid > 0 && ask > 0 && ask >= bid {
		spread = (ask - bid) / last
	}

	if spread <= 0 && config.System.SlippageBPS > 0 {
		spread = config.System.SlippageBPS / 10000
	}

	trail := spread * config.System.TrailSpreadMultiple

	if trail <= 0 {
		return 0.005
	}

	return math.Min(trail, 0.05)
}

func stopFromEntry(entryFill, trail float64) float64 {
	if entryFill <= 0 || trail <= 0 {
		return 0
	}

	return entryFill * (1 - trail)
}

func roundTripFeePct() float64 {
	return cryptoFeePct() * 2
}

func cryptoFeePct() float64 {
	if config.System.TakerFeePct <= 0 {
		return 0
	}

	return config.System.TakerFeePct / 100
}

func minProfitPct(trailPct float64) float64 {
	fees := roundTripFeePct()
	spreadCost := trailPct / math.Max(config.System.TrailSpreadMultiple, 1)

	return fees + spreadCost
}

func pairSymbol(pair asset.Pair) string {
	symbol := pair.Wsname

	if symbol == "" {
		symbol = pair.Altname
	}

	return symbol
}

func sortCandidates(candidates []tradeCandidate) {
	for index := 1; index < len(candidates); index++ {
		for inner := index; inner > 0 && candidateLess(candidates[inner], candidates[inner-1]); inner-- {
			candidates[inner], candidates[inner-1] = candidates[inner-1], candidates[inner]
		}
	}
}

func candidateLess(left, right tradeCandidate) bool {
	leftPriority := typePriority(left.measType)
	rightPriority := typePriority(right.measType)

	if leftPriority != rightPriority {
		return leftPriority > rightPriority
	}

	if left.confidence != right.confidence {
		return left.confidence > right.confidence
	}

	return left.support > right.support
}
