package fluid

import (
	"math"

	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/stats"
)

const (
	GridSize          = 32
	HeightEMAAlpha    = 0.35
	gridQuantileClip  = 0.95
	minSmoothPasses   = 3
	minGridSymbols    = 8
	warmingGridHeight = 0
)

/*
FluidScaleSummary describes field-activity outlier clipping for the UI grid.
*/
type FluidScaleSummary struct {
	ClippedCount int     `json:"clipped_count"`
	ClippedAt    float64 `json:"clipped_at"`
	RawMax       float64 `json:"raw_max"`
	RawMaxSymbol string  `json:"raw_max_symbol,omitempty"`
	DisplayMax   float64 `json:"display_max"`
}

/*
FluidGrid is a UI-ready change-rank × volume-rank height map.
*/
type FluidGrid struct {
	Size        int               `json:"size"`
	Heights     [][]float64       `json:"heights"`
	Min         float64           `json:"min"`
	Max         float64           `json:"max"`
	FilledCells int               `json:"filled_cells"`
	Outliers    FluidScaleSummary `json:"outliers"`
}

/*
GridBuilder maintains EMA-smoothed heights across field snapshots.
*/
type GridBuilder struct {
	smoothedHeights [][]float64
	params          *DisplayParams
}

/*
NewGridBuilder creates an empty fluid grid builder bound to display params.
*/
func NewGridBuilder(params *DisplayParams) *GridBuilder {
	if params == nil {
		params = NewDisplayParams()
	}

	return &GridBuilder{params: params}
}

/*
ResetSmoothing clears EMA state after display parameter changes.
*/
func (builder *GridBuilder) ResetSmoothing() {
	builder.smoothedHeights = nil
}

/*
Build bins symbols by change% × volume rank and returns a display grid.
*/
func (builder *GridBuilder) Build(rows []SymbolSnapshot, size int) FluidGrid {
	if size <= 0 {
		size = builder.params.activeGridSize()
	}

	quantileClip := builder.params.activeQuantileClip()

	heights := newNaNGrid(size)
	cells := newCellGrid(size)

	if len(rows) == 0 {
		return FluidGrid{
			Size:     size,
			Heights:  heights,
			Min:      0,
			Max:      1,
			Outliers: summarizeFluidScaling(nil, quantileClip),
		}
	}

	finiteRows := filterFiniteRows(rows)
	outliers := summarizeFluidScaling(finiteRows, quantileClip)

	if fluidSampledCount(finiteRows) < minGridSymbols {
		builder.ResetSmoothing()

		return warmingGrid(size, fluidSampledCount(finiteRows), outliers)
	}

	if len(finiteRows) == 0 {
		return FluidGrid{
			Size:     size,
			Heights:  heights,
			Min:      0,
			Max:      1,
			Outliers: outliers,
		}
	}

	changes := sortedValues(finiteRows, func(row SymbolSnapshot) float64 {
		return row.ChangePct
	})
	vols := sortedValues(finiteRows, func(row SymbolSnapshot) float64 {
		return row.Vol
	})

	displayValues := make([]float64, len(finiteRows))

	for index, row := range finiteRows {
		displayValues[index] = displayHeight(row, outliers.ClippedAt)
	}

	fallback := stats.Median(displayValues)

	for _, row := range finiteRows {
		if fieldActivity(row) <= 0 {
			continue
		}

		column := binIndex(percentileRank(row.ChangePct, changes), size)
		rowIndex := binIndex(percentileRank(row.Vol, vols), size)
		cells[rowIndex][column] = append(
			cells[rowIndex][column],
			displayHeight(row, outliers.ClippedAt),
		)
	}

	minHeight := math.MaxFloat64
	maxHeight := -math.MaxFloat64

	for rowIndex := 0; rowIndex < size; rowIndex++ {
		for column := 0; column < size; column++ {
			values := cells[rowIndex][column]
			height := math.NaN()

			if len(values) > 0 {
				height = medianPositiveValues(values)
			}

			heights[rowIndex][column] = height

			if !isFinite(height) {
				continue
			}

			minHeight = math.Min(minHeight, height)
			maxHeight = math.Max(maxHeight, height)
		}
	}

	filledCells := smoothEmptyCells(heights, 0)
	smoothed := builder.emaSmoothHeights(heights, size)

	for rowIndex := 0; rowIndex < size; rowIndex++ {
		for column := 0; column < size; column++ {
			heights[rowIndex][column] = smoothed[rowIndex][column]
		}
	}

	if !isFinite(minHeight) || !isFinite(maxHeight) || minHeight == maxHeight {
		minHeight = fallback - 0.5
		maxHeight = fallback + 0.5
	}

	finalizeGridHeights(heights, 0)

	return FluidGrid{
		Size:        size,
		Heights:     heights,
		Min:         minHeight,
		Max:         maxHeight,
		FilledCells: filledCells,
		Outliers:    outliers,
	}
}

func (builder *GridBuilder) emaSmoothHeights(raw [][]float64, size int) [][]float64 {
	if builder.smoothedHeights == nil || len(builder.smoothedHeights) != size {
		builder.smoothedHeights = cloneGrid(raw)

		return builder.smoothedHeights
	}

	for rowIndex := 0; rowIndex < size; rowIndex++ {
		for column := 0; column < size; column++ {
			next := raw[rowIndex][column]
			previous := builder.smoothedHeights[rowIndex][column]

			if math.IsNaN(next) {
				continue
			}

			if math.IsNaN(previous) {
				builder.smoothedHeights[rowIndex][column] = next
				continue
			}

			alpha := builder.params.activeHeightEMAAlpha()
			builder.smoothedHeights[rowIndex][column] = adaptive.BlendEMA(previous, next, alpha)
		}
	}

	return builder.smoothedHeights
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func summarizeFluidScaling(rows []SymbolSnapshot, quantileClip float64) FluidScaleSummary {
	finiteRows := filterFiniteActivityRows(rows)

	if len(finiteRows) == 0 {
		return FluidScaleSummary{}
	}

	if quantileClip <= 0 {
		quantileClip = gridQuantileClip
	}

	activityValues := sortedPositiveValues(finiteRows, func(row SymbolSnapshot) float64 {
		return fieldActivity(row)
	})

	if len(activityValues) == 0 {
		return FluidScaleSummary{}
	}

	clippedAt := math.Max(stats.PercentileSorted(activityValues, quantileClip), 0)

	summary := FluidScaleSummary{
		ClippedAt: clippedAt,
		RawMax:    -math.MaxFloat64,
	}

	for _, row := range finiteRows {
		activity := fieldActivity(row)

		if activity > summary.RawMax {
			summary.RawMax = activity
			summary.RawMaxSymbol = row.Symbol
		}

		if activity > clippedAt {
			summary.ClippedCount++
		}
	}

	summary.DisplayMax = displayActivity(summary.RawMax, clippedAt)

	return summary
}

func displayHeight(row SymbolSnapshot, clippedAt float64) float64 {
	return displayActivity(fieldActivity(row), clippedAt)
}

func fieldActivity(row SymbolSnapshot) float64 {
	activity := math.Abs(row.Re)
	activity = math.Max(activity, math.Abs(row.Div))
	activity = math.Max(activity, math.Abs(row.Vort))
	activity = math.Max(activity, math.Abs(row.Turb))

	return activity
}

func displayActivity(value, clippedAt float64) float64 {
	clamped := value

	if clamped < 0 {
		clamped = 0
	}

	if clippedAt >= 0 && clamped > clippedAt {
		clamped = clippedAt
	}

	return math.Log1p(clamped)
}

func filterFiniteRows(rows []SymbolSnapshot) []SymbolSnapshot {
	filtered := make([]SymbolSnapshot, 0, len(rows))

	for _, row := range rows {
		if !isFinite(row.ChangePct) ||
			!isFinite(row.Vol) ||
			!isFinite(row.Re) ||
			!isFinite(row.Div) ||
			!isFinite(row.Vort) ||
			!isFinite(row.Turb) {
			continue
		}

		filtered = append(filtered, row)
	}

	return filtered
}

func filterFiniteActivityRows(rows []SymbolSnapshot) []SymbolSnapshot {
	filtered := make([]SymbolSnapshot, 0, len(rows))

	for _, row := range rows {
		if !isFinite(row.Re) ||
			!isFinite(row.Div) ||
			!isFinite(row.Vort) ||
			!isFinite(row.Turb) {
			continue
		}

		filtered = append(filtered, row)
	}

	return filtered
}

func sortedValues(
	rows []SymbolSnapshot,
	value func(SymbolSnapshot) float64,
) []float64 {
	values := make([]float64, len(rows))

	for index, row := range rows {
		values[index] = value(row)
	}

	stats.SortFloats(values)

	return values
}

func sortedPositiveValues(
	rows []SymbolSnapshot,
	value func(SymbolSnapshot) float64,
) []float64 {
	values := make([]float64, 0, len(rows))

	for _, row := range rows {
		candidate := value(row)

		if candidate <= 0 {
			continue
		}

		values = append(values, candidate)
	}

	stats.SortFloats(values)

	return values
}

func percentileRank(value float64, sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0.5
	}

	below := 0

	for _, candidate := range sorted {
		if candidate < value {
			below++
		}
	}

	return float64(below) / float64(len(sorted))
}

func fluidSampledCount(rows []SymbolSnapshot) int {
	count := 0

	for _, row := range rows {
		if row.Vol > 0 || row.Visc > 0 {
			count++
		}
	}

	return count
}

func medianPositiveValues(values []float64) float64 {
	positive := make([]float64, 0, len(values))

	for _, value := range values {
		if value <= 0 {
			continue
		}

		positive = append(positive, value)
	}

	if len(positive) == 0 {
		return math.NaN()
	}

	return stats.Median(positive)
}

func binIndex(rank float64, size int) int {
	index := int(math.Floor(rank * float64(size)))

	if index < 0 {
		return 0
	}

	if index >= size {
		return size - 1
	}

	return index
}

func newNaNGrid(size int) [][]float64 {
	grid := make([][]float64, size)

	for rowIndex := range grid {
		grid[rowIndex] = make([]float64, size)

		for column := 0; column < size; column++ {
			grid[rowIndex][column] = math.NaN()
		}
	}

	return grid
}

func newCellGrid(size int) [][][]float64 {
	grid := make([][][]float64, size)

	for rowIndex := range grid {
		grid[rowIndex] = make([][]float64, size)

		for column := 0; column < size; column++ {
			grid[rowIndex][column] = make([]float64, 0, 4)
		}
	}

	return grid
}

func cloneGrid(source [][]float64) [][]float64 {
	cloned := make([][]float64, len(source))

	for rowIndex, row := range source {
		cloned[rowIndex] = append([]float64(nil), row...)
	}

	return cloned
}

func smoothEmptyCells(grid [][]float64, fallback float64) int {
	size := len(grid)
	filled := 0

	for rowIndex := 0; rowIndex < size; rowIndex++ {
		for column := 0; column < size; column++ {
			if isFinite(grid[rowIndex][column]) {
				filled++
			}
		}
	}

	if filled == 0 {
		for rowIndex := 0; rowIndex < size; rowIndex++ {
			for column := 0; column < size; column++ {
				grid[rowIndex][column] = fallback
			}
		}

		return 0
	}

	passes := minSmoothPasses

	if size > passes {
		passes = size
	}

	for pass := 0; pass < passes; pass++ {
		for rowIndex := 0; rowIndex < size; rowIndex++ {
			for column := 0; column < size; column++ {
				if isFinite(grid[rowIndex][column]) {
					continue
				}

				sum := 0.0
				count := 0

				for deltaRow := -1; deltaRow <= 1; deltaRow++ {
					for deltaColumn := -1; deltaColumn <= 1; deltaColumn++ {
						if deltaRow == 0 && deltaColumn == 0 {
							continue
						}

						neighborRow := rowIndex + deltaRow
						neighborColumn := column + deltaColumn

						if neighborRow < 0 || neighborRow >= size ||
							neighborColumn < 0 || neighborColumn >= size {
							continue
						}

						value := grid[neighborRow][neighborColumn]

						if !isFinite(value) {
							continue
						}

						sum += value
						count++
					}
				}

				if count > 0 {
					grid[rowIndex][column] = sum / float64(count)
				}
			}
		}
	}

	for rowIndex := 0; rowIndex < size; rowIndex++ {
		for column := 0; column < size; column++ {
			if isFinite(grid[rowIndex][column]) {
				continue
			}

			grid[rowIndex][column] = fallback
		}
	}

	return filled
}

func finalizeGridHeights(heights [][]float64, fallback float64) {
	if !isFinite(fallback) {
		fallback = 0
	}

	for rowIndex := range heights {
		for column := range heights[rowIndex] {
			if isFinite(heights[rowIndex][column]) {
				continue
			}

			heights[rowIndex][column] = fallback
		}
	}
}

func warmingGrid(size, sampled int, outliers FluidScaleSummary) FluidGrid {
	heights := newFlatGrid(size, warmingGridHeight)

	return FluidGrid{
		Size:        size,
		Heights:     heights,
		Min:         0,
		Max:         warmingGridHeight,
		FilledCells: sampled,
		Outliers:    outliers,
	}
}

func newFlatGrid(size int, height float64) [][]float64 {
	grid := make([][]float64, size)

	for rowIndex := range grid {
		grid[rowIndex] = make([]float64, size)

		for column := 0; column < size; column++ {
			grid[rowIndex][column] = height
		}
	}

	return grid
}
