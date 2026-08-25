package types

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	RelativeSpread  float64   `json:"relativeSpread"`
}

/*
CrossSection contains the retained peer metrics for the central market feed.
Measure upserts by symbol; Breadth and Leadership observe the live cohort.
*/
type CrossSection struct {
	Metrics *sync.Map `json:"metrics"`
}

/*
NewCrossSection creates empty cross-sectional state for the market feed.
*/
func NewCrossSection() *CrossSection {
	return &CrossSection{
		Metrics: &sync.Map{},
	}
}

/*
Measure calculates current peer liquidity and change metrics from ticker rows.
Later observations replace the same symbol rather than weighting it twice.
*/
func (crossSection *CrossSection) Measure(rows []kraken.TickerData) {
	for _, row := range rows {
		relativeSpread := 0.0

		if row.Bid != nil && row.Ask != nil {
			midpoint := (row.Bid.Float64() + row.Ask.Float64()) / 2

			if midpoint > 0 {
				relativeSpread = (row.Ask.Float64() - row.Bid.Float64()) / midpoint
			}
		}

		metric := SymbolMetric{
			Symbol:          row.Symbol,
			At:              row.Timestamp,
			Volume:          row.Volume,
			QuoteNotional:   crossSection.QuoteNotional(row),
			ExecutableDepth: crossSection.ExecutableDepth(row),
			LatestChange:    row.ChangePct / 100,
			RelativeSpread:  relativeSpread,
		}

		crossSection.Metrics.Store(row.Symbol, metric)
	}
}

/*
Breadth calculates the fraction of current symbols with positive change.
*/
func (crossSection *CrossSection) Breadth() float64 {
	positive := 0
	count := 0

	crossSection.Metrics.Range(func(_, value any) bool {
		if value.(SymbolMetric).LatestChange > 0 {
			positive++
		}

		count++
		return true
	})

	if count == 0 {
		return 0
	}

	return float64(positive) / float64(count)
}

/*
Leadership returns the symbol with the greatest current absolute change when
it exceeds the cohort's median absolute change.
*/
func (crossSection *CrossSection) Leadership() (string, float64) {
	metrics := make([]SymbolMetric, 0)

	crossSection.Metrics.Range(func(_, value any) bool {
		metrics = append(metrics, value.(SymbolMetric))
		return true
	})
	sort.Slice(metrics, func(left, right int) bool {
		return metrics[left].Symbol < metrics[right].Symbol
	})
	changes := make([]float64, 0, len(metrics))
	leader := ""
	leaderChange := 0.0

	for _, metric := range metrics {
		change := math.Abs(metric.LatestChange)
		changes = append(changes, change)

		if change > leaderChange {
			leader = metric.Symbol
			leaderChange = change
		}

	}

	threshold, ok := statistic.MedianAbsoluteOf(changes)

	if !ok || leaderChange <= threshold {
		return "", threshold
	}

	return leader, threshold
}

/*
QuoteNotional values reported volume at VWAP, or at the latest trade when the
exchange has not yet published VWAP.
*/
func (crossSection *CrossSection) QuoteNotional(row kraken.TickerData) float64 {
	rate := row.Vwap

	if rate <= 0 {
		if row.Last == nil {
			return 0
		}

		rate = row.Last.Float64()
	}

	return math.Max(0, row.Volume*rate)
}

/*
ExecutableDepth values the smaller side of the current top-of-book quantity at
the midpoint because only matched two-sided depth is immediately executable.
*/
func (crossSection *CrossSection) ExecutableDepth(row kraken.TickerData) float64 {
	if row.Bid == nil || row.Ask == nil {
		return 0
	}

	return math.Max(0, math.Min(
		row.BidQty, row.AskQty,
	)*(row.Bid.Float64()+row.Ask.Float64())/2)
}

/*
MarshalJSON encodes peer metrics as a JSON array so thesis checkpoints and UI
frames keep a stable wire shape.
*/
func (crossSection *CrossSection) MarshalJSON() ([]byte, error) {
	metrics := make([]SymbolMetric, 0)

	if crossSection != nil && crossSection.Metrics != nil {
		crossSection.Metrics.Range(func(_, value any) bool {
			metrics = append(metrics, value.(SymbolMetric))
			return true
		})
	}

	return sonic.Marshal(struct {
		Metrics []SymbolMetric `json:"metrics"`
	}{
		Metrics: metrics,
	})
}

/*
UnmarshalJSON restores peer metrics into the concurrent map from the array wire
shape used by thesis checkpoints.
*/
func (crossSection *CrossSection) UnmarshalJSON(data []byte) error {
	var wire struct {
		Metrics []SymbolMetric `json:"metrics"`
	}

	if err := sonic.Unmarshal(data, &wire); err != nil {
		return err
	}

	crossSection.Metrics = &sync.Map{}

	for _, metric := range wire.Metrics {
		crossSection.Metrics.Store(metric.Symbol, metric)
	}

	return nil
}
