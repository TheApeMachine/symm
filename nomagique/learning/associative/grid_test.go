package associative_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
)

type testCell struct {
	*core.PrimitiveError
	address *geometry.Coordinate
	obs     *statistic.Observation[*geometry.Coordinate]
}

func newTestCell(x, y int, movement, authority float64) *testCell {
	coord := geometry.NewCoordinate(x, y)
	return &testCell{
		PrimitiveError: core.NewPrimitiveError(),
		address:        coord,
		obs:            statistic.NewObservation(coord, movement, authority, 1.0),
	}
}

func (cell *testCell) Identity() *geometry.Coordinate { return cell.address }

func (cell *testCell) Identify(addr *geometry.Coordinate) core.Identifiable[*geometry.Coordinate] {
	cell.address = addr
	return cell
}

func (cell *testCell) Connect(core.Primitive) {}

func (cell *testCell) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(cell.obs))
	}
}

func TestAssociativeGridPipeline(t *testing.T) {
	Convey("Associative Grid measures sympathy before spatial remapping and segments regions via wrapped composition", t, func() {
		// Cell A at (0, 0): authority 1.0 (strong hot spot), movement +0.5
		// Cell Mid at (1, 0): authority 0.25 (weak border cell), movement +0.5
		// Cell B at (2, 0): authority 1.0 (strong hot spot), movement +0.5
		cellA := newTestCell(0, 0, 0.5, 1.0)
		cellMid := newTestCell(1, 0, 0.5, 0.25)
		cellB := newTestCell(2, 0, 0.5, 1.0)

		rawGrid := store.NewGrid(cellA, cellMid, cellB)
		grid := associative.NewGrid(rawGrid)

		So(grid.Error(), ShouldBeNil)

		var latestEdges []*geometry.Edge

		// Execute each cell sequentially through the grid pipeline
		for _, cell := range []*testCell{cellA, cellMid, cellB} {
			query := store.NewQuery[*geometry.Coordinate, any](cell, core.Execute)

			latestEdges = nil
			for result := range grid.Next(query.Next(nil)) {
				latestEdges = append(latestEdges, (*geometry.Edge)(result))
			}
		}

		So(grid.Error(), ShouldBeNil)
		So(len(latestEdges), ShouldBeGreaterThan, 0)

		// Verification 1: Cell coordinates in store remain unchanged (immutable identity)
		So(cellA.Identity().X, ShouldEqual, 0)
		So(cellA.Identity().Y, ShouldEqual, 0)
		So(cellMid.Identity().X, ShouldEqual, 1)
		So(cellMid.Identity().Y, ShouldEqual, 0)
		So(cellB.Identity().X, ShouldEqual, 2)
		So(cellB.Identity().Y, ShouldEqual, 0)

		// Verification 2: Sympathetic attraction and distance
		for _, edge := range latestEdges {
			So(edge.Distance, ShouldBeGreaterThan, 0)
		}
	})
}
