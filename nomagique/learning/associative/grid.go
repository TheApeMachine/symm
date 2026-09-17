package associative

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Grid composes a query-producing primitive with the distributed store and its
output pipeline. Routing can itself be a composition. Metric state stays with
registered owners; the grid does not materialize or copy their values.

Region segmentation is purely composed from atomic transformations:
- store.Grid[T]: coordinate-addressed distributed cell store
- statistic.Sympathy[T]: evaluates directional concordance and relative magnitude to find sympathetic metrics
- geometry.Mapping[T]: wraps the spatial pipeline, maps addresses to virtual 2D coordinates, leaving store coordinates immutable
  - geometry.Inversion: sympathy to metric distance inversion
  - geometry.Relaxation: authority-weighted stress displacement forming hot spots
  - geometry.Forest: minimum spanning forest expansion
  - geometry.Peak: climbs spanning forest to local density peaks
  - geometry.Border: saddle-point border detection where distinct basins meet
*/
type Grid[T interface {
	core.Ordered[T]
	comparable
}] struct {
	*core.PrimitiveError
	pipeline *nomagique.Number
}

func NewGrid[T interface {
	core.Ordered[T]
	comparable
}](grid *store.Grid[T]) *Grid[T] {
	return &Grid[T]{
		PrimitiveError: core.NewPrimitiveError(),
		pipeline: nomagique.NewNumber(
			grid,
			statistic.NewSympathy[T](),
			geometry.NewMapping[T](
				geometry.NewInversion(),
				geometry.NewRelaxation(),
				geometry.NewForest(),
				geometry.NewPeak(),
				geometry.NewBorder(),
			),
		),
	}
}

func (grid *Grid[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return grid.pipeline.Next(in)
}
