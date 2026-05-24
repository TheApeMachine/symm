package fluid

import (
	"math"

	"github.com/theapemachine/symm/engine"
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
	Grid        FluidGrid        `json:"grid"`
}

/*
FieldAggregate holds cross-section-normalized fluid scalars for the dashboard.
Each value is median/MAD activity for that metric across sampled symbols.
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
	rows := fluid.track.SnapshotRows(fluid.Symbols(), fluid.Ingest().Ticker())
	aggregate, sampledCount := aggregateFieldRows(rows)

	return FieldSnapshot{
		SymbolCount: sampledCount,
		Field:       aggregate,
		Symbols:     rows,
		Grid:        fluid.gridBuilder.Build(rows, GridSize),
	}
}

func aggregateFieldRows(rows []SymbolSnapshot) (FieldAggregate, int) {
	divValues := make([]float64, 0, len(rows))
	vortValues := make([]float64, 0, len(rows))
	turbValues := make([]float64, 0, len(rows))
	viscValues := make([]float64, 0, len(rows))
	reValues := make([]float64, 0, len(rows))

	for _, row := range rows {
		if !rowHasFluidSample(row) {
			continue
		}

		divValues = append(divValues, row.Div)
		vortValues = append(vortValues, row.Vort)
		turbValues = append(turbValues, row.Turb)
		viscValues = append(viscValues, row.Visc)
		reValues = append(reValues, row.Re)
	}

	sampledCount := len(divValues)

	if sampledCount == 0 {
		return FieldAggregate{}, 0
	}

	return FieldAggregate{
		Div:  robustCrossSectionActivity(divValues),
		Vort: robustCrossSectionActivity(vortValues),
		Turb: robustCrossSectionActivity(turbValues),
		Visc: robustCrossSectionActivity(viscValues),
		Re:   robustCrossSectionActivity(reValues),
	}, sampledCount
}

func rowHasFluidSample(row SymbolSnapshot) bool {
	return row.Vol > 0 || row.Visc > 0
}

/*
SnapshotRows returns per-symbol fluid rows for telemetry.
*/
func (trackStore *TrackStore) SnapshotRows(
	symbols []string,
	ticker *kticker.Ticker,
) []SymbolSnapshot {
	trackStore.shard.RLockMap()
	defer trackStore.shard.RUnlockMap()

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
			Vol:       sample.density,
			Div:       source,
			Vort:      velocity,
			Turb:      shock,
			Visc:      sample.viscosity,
			Re:        reynolds,
		})
	}

	return rows
}

/*
SymbolRisk returns the latest fluid topology metrics for one symbol.
*/
func (trackStore *TrackStore) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	trackStore.shard.RLockMap()
	defer trackStore.shard.RUnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.samples) == 0 {
		return engine.SymbolRisk{}, false
	}

	sample := track.samples[len(track.samples)-1]
	prior := track.lastSample

	if len(track.samples) >= 2 {
		prior = track.samples[len(track.samples)-2]
	}

	turbulence := 0.0

	if track.hasPrior {
		turbulence = burgersShock(sample, prior)
	}

	reynolds := 0.0

	if sample.viscosity > 0 {
		reynolds = math.Abs(sample.velocity) * sample.density / sample.viscosity
	}

	return engine.SymbolRisk{
		Reynolds:   reynolds,
		Turbulence: turbulence,
	}, true
}
