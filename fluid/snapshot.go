package fluid

import (
	"math"

	kticker "github.com/theapemachine/symm/kraken/ticker"
)

/*
SymbolSnapshot is one row in the fluid field UI grid.
*/
type SymbolSnapshot struct {
	Symbol    string  `json:"symbol"`
	ChangePct float64 `json:"change_pct"`
	Vol       float64 `json:"vol"`
	Div       float64 `json:"div"`
	Vort      float64 `json:"vort"`
	Turb      float64 `json:"turb"`
	Visc      float64 `json:"visc"`
	Re        float64 `json:"re"`
}

/*
FieldSnapshot aggregates cross-section fluid metrics for the dashboard.
*/
type FieldSnapshot struct {
	SymbolCount int              `json:"symbol_count"`
	Field       FieldAggregate   `json:"field"`
	Symbols     []SymbolSnapshot `json:"symbols"`
}

/*
FieldAggregate holds aggregate fluid scalars for the surface chart.
*/
type FieldAggregate struct {
	Div  float64 `json:"div"`
	Vort float64 `json:"vort"`
	Turb float64 `json:"turb"`
	Visc float64 `json:"visc"`
	Re   float64 `json:"re"`
}

/*
FieldSnapshot builds a UI field snapshot from the latest per-symbol samples.
*/
func (fluid *Fluid) FieldSnapshot() FieldSnapshot {
	rows := fluid.track.SnapshotRows(fluid.symbols, fluid.ticker)
	aggregate := FieldAggregate{}

	if len(rows) == 0 {
		return FieldSnapshot{Symbols: rows}
	}

	for _, row := range rows {
		aggregate.Div += row.Div
		aggregate.Vort += row.Vort
		aggregate.Turb += row.Turb
		aggregate.Visc += row.Visc
		aggregate.Re += row.Re
	}

	count := float64(len(rows))
	aggregate.Div /= count
	aggregate.Vort /= count
	aggregate.Turb /= count
	aggregate.Visc /= count
	aggregate.Re /= count

	return FieldSnapshot{
		SymbolCount: len(rows),
		Field:       aggregate,
		Symbols:     rows,
	}
}

/*
SnapshotRows returns per-symbol fluid rows for telemetry.
*/
func (trackStore *TrackStore) SnapshotRows(
	symbols []string,
	ticker *kticker.Ticker,
) []SymbolSnapshot {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	rows := make([]SymbolSnapshot, 0, len(symbols))

	for _, symbol := range symbols {
		track, ok := trackStore.bySymbol[symbol]

		changePct := 0.0

		if ticker != nil {
			if _, _, _, change, quoteOK := ticker.Quote(symbol); quoteOK {
				changePct = change
			}
		}

		if !ok || len(track.samples) == 0 {
			if ok && track.dailyQuoteVol > 0 {
				rows = append(rows, SymbolSnapshot{
					Symbol:    symbol,
					ChangePct: changePct,
				})
			}

			continue
		}

		sample := track.samples[len(track.samples)-1]
		prior := track.lastSample

		if len(track.samples) >= 2 {
			prior = track.samples[len(track.samples)-2]
		}

		source := 0.0
		shock := 0.0

		if track.hasPrior {
			source = continuitySource(sample, prior)
			shock = burgersShock(sample, prior)
		}

		velocity := math.Abs(sample.velocity)
		reynolds := 0.0

		if sample.viscosity > 0 {
			reynolds = velocity * sample.density / sample.viscosity
		}

		rows = append(rows, SymbolSnapshot{
			Symbol:    symbol,
			ChangePct: changePct,
			Vol:    sample.density,
			Div:    source,
			Vort:   velocity,
			Turb:   shock,
			Visc:   sample.viscosity,
			Re:     reynolds,
		})
	}

	return rows
}
