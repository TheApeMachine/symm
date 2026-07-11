package types

import (
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
)

const (
	defaultReturnCap  = 64
	defaultMinBars    = 4
	defaultBreadthCap = 128
)

type CrossSectionConfig struct {
	ReturnCap  int
	MinBars    int
	BreadthCap int
}

type CrossSection struct {
	config  CrossSectionConfig
	symbols map[string][]kraken.TickerData
}

func DefaultCrossSectionConfig() CrossSectionConfig {
	return CrossSectionConfig{
		ReturnCap:  defaultReturnCap,
		MinBars:    defaultMinBars,
		BreadthCap: defaultBreadthCap,
	}
}

func NewCrossSection(config CrossSectionConfig) (*CrossSection, error) {
	if config.ReturnCap <= 0 {
		return nil, errnie.Err(errnie.Validation, "types: cross-section return cap must be positive", nil)
	}

	if config.MinBars <= 1 {
		return nil, errnie.Err(errnie.Validation, "types: cross-section min bars must exceed one", nil)
	}

	if config.BreadthCap <= 0 {
		return nil, errnie.Err(errnie.Validation, "types: cross-section breadth cap must be positive", nil)
	}

	return &CrossSection{
		config:  config,
		symbols: map[string][]kraken.TickerData{},
	}, nil
}

func (crossSection *CrossSection) Observe(rows []kraken.TickerData) error {
	if crossSection == nil {
		return errnie.Err(errnie.Validation, "types: cross-section is nil", nil)
	}

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)
		lastPrice := row.Last.Float64()

		if symbol == "" || lastPrice <= 0 || row.Timestamp.IsZero() {
			continue
		}

		observations := append(crossSection.symbols[symbol], row)
		if len(observations) > crossSection.config.ReturnCap+1 {
			observations = observations[len(observations)-crossSection.config.ReturnCap-1:]
		}

		crossSection.symbols[symbol] = observations
	}

	return nil
}

/*
Snapshot returns an independent, deep copy of crossSection's current
per-symbol history. The result never changes even as the source keeps
observing new ticker rows, so every signal measuring events within the
same watermark can share one frozen cross-sectional view instead of
racing a live, mutating accumulator or peer signals seeing inconsistent
partial history.
*/
func (crossSection *CrossSection) Snapshot() *CrossSection {
	if crossSection == nil {
		return nil
	}

	symbols := make(map[string][]kraken.TickerData, len(crossSection.symbols))

	for symbol, observations := range crossSection.symbols {
		symbols[symbol] = append([]kraken.TickerData(nil), observations...)
	}

	return &CrossSection{config: crossSection.config, symbols: symbols}
}

func (crossSection *CrossSection) Volumes() []float64 {
	volumes := make([]float64, 0, len(crossSection.symbols))
	for _, observations := range crossSection.symbols {
		latest, ok := latestTicker(observations)
		if !ok || latest.Volume <= 0 {
			continue
		}

		volumes = append(volumes, latest.Volume)
	}

	return volumes
}

/*
QuoteNotionals returns each symbol's latest executable quote notional
(volume times the vwap traded rate, falling back to last trade price
when no vwap is reported yet), the same tradable-value axis Universe
ranking uses. Comparing this instead of raw base-currency Volume avoids
mixing symbols denominated in wildly different unit sizes.
*/
func (crossSection *CrossSection) QuoteNotionals() []float64 {
	notionals := make([]float64, 0, len(crossSection.symbols))

	for _, observations := range crossSection.symbols {
		latest, ok := latestTicker(observations)

		if !ok {
			continue
		}

		if notional := QuoteNotional(latest); notional > 0 {
			notionals = append(notionals, notional)
		}
	}

	return notionals
}

/*
ExecutableDepths returns each symbol's latest executable top-of-book
depth: the smaller of bid/ask quantity, valued at the mid price. This is
liquidity actually available to trade right now, not a volume proxy for
it.
*/
func (crossSection *CrossSection) ExecutableDepths() []float64 {
	depths := make([]float64, 0, len(crossSection.symbols))

	for _, observations := range crossSection.symbols {
		latest, ok := latestTicker(observations)

		if !ok {
			continue
		}

		if depth := ExecutableDepth(latest); depth > 0 {
			depths = append(depths, depth)
		}
	}

	return depths
}

/*
QuoteNotional values row's traded volume at its vwap rate, falling back
to the last trade price when vwap has not been reported yet. Zero when
either input is unavailable.
*/
func QuoteNotional(row kraken.TickerData) float64 {
	rate := row.Vwap

	if rate <= 0 {
		rate = row.Last.Float64()
	}

	if rate <= 0 || row.Volume <= 0 {
		return 0
	}

	return row.Volume * rate
}

/*
ExecutableDepth values the smaller of row's bid/ask quantity at the mid
price: the quantity a taker could actually clear on either side right
now without walking past the top of book. Zero when the quote is not
two-sided.
*/
func ExecutableDepth(row kraken.TickerData) float64 {
	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= 0 {
		return 0
	}

	qty := min(row.BidQty, row.AskQty)

	if qty <= 0 {
		return 0
	}

	return qty * (bid + ask) / 2
}

func (crossSection *CrossSection) Breadth() float64 {
	var positive float64
	var total float64

	for _, observations := range crossSection.symbols {
		change, ok := latestChange(observations)
		if !ok {
			continue
		}

		total++
		if change > 0 {
			positive++
		}
	}

	if total == 0 {
		return 0
	}

	return positive / total
}

func (crossSection *CrossSection) IsLeader(symbol string, change float64) bool {
	leader := crossSection.Leader()
	if leader != strings.TrimSpace(symbol) {
		return false
	}

	threshold := crossSection.LeadershipThreshold()
	if threshold <= 0 {
		return false
	}

	return math.Abs(change) >= threshold
}

func (crossSection *CrossSection) LeadershipThreshold() float64 {
	changes := make([]float64, 0, len(crossSection.symbols))
	for _, observations := range crossSection.symbols {
		if len(observations) < crossSection.config.MinBars {
			continue
		}

		change, ok := latestChange(observations)
		if ok {
			changes = append(changes, math.Abs(change))
		}
	}

	if len(changes) == 0 {
		return 0
	}

	sort.Float64s(changes)
	return changes[len(changes)/2]
}

func (crossSection *CrossSection) Leader() string {
	var leader string
	var strength float64

	for symbol, observations := range crossSection.symbols {
		change, ok := latestChange(observations)
		if !ok || math.Abs(change) <= strength {
			continue
		}

		leader = symbol
		strength = math.Abs(change)
	}

	return leader
}

func (crossSection *CrossSection) MaxReturnWindow() int {
	if crossSection == nil || crossSection.config.ReturnCap <= 0 {
		return 0
	}

	return crossSection.config.ReturnCap
}

func (crossSection *CrossSection) Symbols() []string {
	symbols := make([]string, 0, len(crossSection.symbols))
	for symbol := range crossSection.symbols {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	return symbols
}

func (crossSection *CrossSection) SymbolReturns(symbol string, window int) []float64 {
	observations := crossSection.window(symbol, window+1)
	return tickerReturns(observations)
}

func (crossSection *CrossSection) SymbolSamples(
	symbol string,
	window int,
) []nomcorrelation.Sample {
	observations := crossSection.window(symbol, window)
	samples := make([]nomcorrelation.Sample, 0, len(observations))

	for _, row := range observations {
		lastPrice := row.Last.Float64()

		if lastPrice <= 0 || row.Timestamp.IsZero() {
			continue
		}

		samples = append(samples, nomcorrelation.Sample{
			At:    row.Timestamp,
			Value: lastPrice,
		})
	}

	return samples
}

func (crossSection *CrossSection) window(symbol string, window int) []kraken.TickerData {
	observations := crossSection.symbols[strings.TrimSpace(symbol)]
	if window <= 0 || len(observations) <= window {
		return append([]kraken.TickerData(nil), observations...)
	}

	return append([]kraken.TickerData(nil), observations[len(observations)-window:]...)
}

func latestTicker(rows []kraken.TickerData) (kraken.TickerData, bool) {
	if len(rows) == 0 {
		return kraken.TickerData{}, false
	}

	return rows[len(rows)-1], true
}

func latestChange(rows []kraken.TickerData) (float64, bool) {
	latest, ok := latestTicker(rows)
	if !ok {
		return 0, false
	}

	if latest.ChangePct != 0 {
		return latest.ChangePct / 100, true
	}

	if len(rows) < 2 {
		return 0, false
	}

	previous := rows[len(rows)-2]
	previousLast := previous.Last.Float64()
	latestLast := latest.Last.Float64()

	if previousLast <= 0 || latestLast <= 0 {
		return 0, false
	}

	return math.Log(latestLast / previousLast), true
}

func tickerReturns(rows []kraken.TickerData) []float64 {
	if len(rows) < 2 {
		return nil
	}

	returns := make([]float64, 0, len(rows)-1)
	for index := 1; index < len(rows); index++ {
		previous := rows[index-1]
		current := rows[index]
		previousLast := previous.Last.Float64()
		currentLast := current.Last.Float64()

		if previousLast <= 0 || currentLast <= 0 {
			continue
		}

		returns = append(returns, math.Log(currentLast/previousLast))
	}

	return returns
}
