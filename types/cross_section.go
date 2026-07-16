package types

import (
	"math"
	"strings"
	"time"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
)

/*
SymbolMetric contains one symbol's current cross-sectional values. It caches
the numerical projection signals compare so Kraken decimal conversion is paid
once per tick.
*/
type SymbolMetric struct {
	Symbol          string    `json:"symbol"`
	At              time.Time `json:"at"`
	Volume          float64   `json:"volume"`
	QuoteNotional   float64   `json:"quoteNotional"`
	ExecutableDepth float64   `json:"executableDepth"`
	LatestChange    float64   `json:"latestChange"`
}

/*
CrossSection contains the current tick's peer metrics. Each signal measures an
isolated view, then Planner merges those views into the completed Thesis.
*/
type CrossSection struct {
	Metrics []SymbolMetric `json:"metrics"`
	index   map[string]int
}

/*
NewCrossSection creates empty cross-sectional state for one Thesis tick.
*/
func NewCrossSection() *CrossSection {
	return &CrossSection{
		Metrics: make([]SymbolMetric, 0),
		index:   make(map[string]int),
	}
}

/*
Measure calculates current peer liquidity and change metrics from ticker rows.
Later observations replace the same symbol rather than weighting it twice.
*/
func (crossSection *CrossSection) Measure(rows []kraken.TickerData) {
	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		metric := SymbolMetric{
			Symbol:          symbol,
			At:              row.Timestamp,
			Volume:          row.Volume,
			QuoteNotional:   QuoteNotional(row),
			ExecutableDepth: ExecutableDepth(row),
			LatestChange:    row.ChangePct / 100,
		}
		index, exists := crossSection.index[symbol]

		if exists {
			if !row.Timestamp.After(crossSection.Metrics[index].At) {
				continue
			}

			crossSection.Metrics[index] = metric

			continue
		}

		crossSection.index[symbol] = len(crossSection.Metrics)
		crossSection.Metrics = append(crossSection.Metrics, metric)
	}
}

/*
Merge retains the newest metric for each symbol from an independently measured
cross-section so Planner can combine concurrent signal results deterministically.
*/
func (crossSection *CrossSection) Merge(incoming *CrossSection) {
	if incoming == nil {
		return
	}

	for _, metric := range incoming.Metrics {
		index, exists := crossSection.index[metric.Symbol]

		if exists {
			if metric.At.After(crossSection.Metrics[index].At) {
				crossSection.Metrics[index] = metric
			}

			continue
		}

		crossSection.index[metric.Symbol] = len(crossSection.Metrics)
		crossSection.Metrics = append(crossSection.Metrics, metric)
	}
}

/*
Breadth calculates the fraction of current symbols with positive change.
*/
func (crossSection *CrossSection) Breadth() float64 {
	if len(crossSection.Metrics) == 0 {
		return 0
	}

	positive := 0

	for _, metric := range crossSection.Metrics {
		if metric.LatestChange > 0 {
			positive++
		}
	}

	return float64(positive) / float64(len(crossSection.Metrics))
}

/*
Leadership returns the symbol with the greatest current absolute change when
it exceeds the cohort's median absolute change.
*/
func (crossSection *CrossSection) Leadership() (string, float64) {
	if len(crossSection.Metrics) < 2 {
		return "", 0
	}

	changes := make([]float64, len(crossSection.Metrics))
	leader := ""
	leaderChange := 0.0

	for index, metric := range crossSection.Metrics {
		change := math.Abs(metric.LatestChange)
		changes[index] = change

		if change > leaderChange {
			leader = metric.Symbol
			leaderChange = change
		}
	}

	threshold, ok := statistic.MedianAbsoluteOf(changes)

	if !ok || threshold <= 0 || leaderChange <= threshold {
		return "", threshold
	}

	return leader, threshold
}

/*
QuoteNotional values reported volume at VWAP, or at the latest trade when the
exchange has not yet published VWAP.
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
ExecutableDepth values the smaller side of the current top-of-book quantity at
the midpoint because only matched two-sided depth is immediately executable.
*/
func ExecutableDepth(row kraken.TickerData) float64 {
	if row.Bid == nil || row.Ask == nil {
		return 0
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= 0 {
		return 0
	}

	quantity := min(row.BidQty, row.AskQty)

	if quantity <= 0 {
		return 0
	}

	return quantity * (bid + ask) / 2
}
