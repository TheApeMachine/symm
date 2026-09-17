package store_test

import (
	"errors"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/transport"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNewGrid(t *testing.T) {
	Convey("Construction registers independent primitives within each entity", t, func() {
		first := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(1.0)}
		second := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(1, 1), Primitive: store.NewRetained(2.0)}
		other := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(3.0)}
		grid := store.NewGrid(
			map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": first, "ETH/USD": other},
			map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": second},
		)
		So(grid.Error(), ShouldBeNil)
		So(first.Identity().X, ShouldEqual, 0)
		So(second.Identity().X, ShouldEqual, 1)
		So(other.Identity().X, ShouldEqual, 0)
		Convey("A query follows its typed subject's current identity", func() {
			subject := store.NewQuery[*geometry.Coordinate, int](nil, data.ActionNone)
			subject.Address = first.Identity()
			query := store.NewQuery[*geometry.Coordinate, float64](subject, data.ActionRead)
			query.Entity = "BTC/USD"
			So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, first)
			subject.Identify(second.Identity())
			So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, second)
		})
		for _, fixture := range []struct {
			entity string
			member *tests.Member[*geometry.Coordinate]
		}{
			{"BTC/USD", first}, {"BTC/USD", second}, {"ETH/USD", other},
		} {
			query := store.NewQuery[*geometry.Coordinate, float64](nil, data.ActionRead)
			query.Entity = fixture.entity
			// A distinct pointer at the same geometric position must resolve the cell.
			query.Address = geometry.NewCoordinate(fixture.member.Identity().X, fixture.member.Identity().Y)
			So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, fixture.member)
		}
	})
}

func TestGridNext(t *testing.T) {
	Convey("Grid handles addressed operations without owning the member's value", t, func() {
		resident := store.NewRetained(0.0)
		member := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: resident}
		grid := store.NewGrid(map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": member})
		query := store.NewQuery[*geometry.Coordinate, float64](nil, data.ActionRead)
		query.Entity, query.Address = "BTC/USD", member.Identity()

		Convey("Read returns the same member without executing it", func() {
			So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, member)
			So(sequence.Read[float64](resident.Next(nil)), ShouldEqual, 0)
		})
		Convey("Execute preserves payload order and state across repeated runs", func() {
			query.Actionable = data.ActionExecute
			for _, values := range [][]float64{{1, 4, -2}, {0, 8}} {
				So(tests.CollectSeq[float64](grid.Next(query.Next(sequence.NewValue(values...)))), ShouldResemble, values)
				So(sequence.Read[float64](resident.Next(nil)), ShouldEqual, values[len(values)-1])
			}
			So(grid.Error(), ShouldBeNil)
		})
		Convey("Early stop does not execute later arrivals", func() {
			query.Actionable = data.ActionExecute
			for range grid.Next(query.Next(sequence.NewValue(5.0, 9.0))) {
				break
			}
			So(sequence.Read[float64](resident.Next(nil)), ShouldEqual, 5)
		})
		Convey("An identify query registers a new entity", func() {
			incoming := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(3.0)}
			identify := store.NewQuery[*geometry.Coordinate, core.Identifiable[*geometry.Coordinate]](nil, data.ActionIdentify, sequence.NewValue[core.Identifiable[*geometry.Coordinate]](incoming))
			incoming.Identify(geometry.NewCoordinate(4, -2))
			identify.Entity, identify.Address = "ETH/USD", incoming.Identity()
			So(sequence.Read[core.Primitive](grid.Next(identify.Next(nil))), ShouldEqual, incoming)
			So(incoming.Identity(), ShouldEqual, identify.Address)
		})
		Convey("Registration cannot overwrite an occupied address", func() {
			incoming := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(3.0)}
			identify := store.NewQuery[*geometry.Coordinate, core.Identifiable[*geometry.Coordinate]](nil, data.ActionIdentify, sequence.NewValue[core.Identifiable[*geometry.Coordinate]](incoming))
			identify.Entity, identify.Address = query.Entity, query.Address
			So(tests.CollectSeq[core.Primitive](grid.Next(identify.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
			So(incoming.Identity().X, ShouldEqual, 0)
		})
		Convey("A member cannot acquire a second address", func() {
			identify := store.NewQuery[*geometry.Coordinate, core.Identifiable[*geometry.Coordinate]](nil, data.ActionIdentify, sequence.NewValue[core.Identifiable[*geometry.Coordinate]](member))
			identify.Entity, identify.Address = query.Entity, geometry.NewCoordinate(8, 8)
			So(tests.CollectSeq[core.Primitive](grid.Next(identify.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
			So(member.Identity(), ShouldEqual, query.Address)
		})
		Convey("Both coordinate dimensions participate in lookup", func() {
			query.Address = geometry.NewCoordinate(0, 1)
			So(tests.CollectSeq[core.Primitive](grid.Next(query.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrNotHeld), ShouldBeTrue)
		})
		Convey("Unknown entities are explicit failures", func() {
			query.Entity = "missing"
			So(tests.CollectSeq[core.Primitive](grid.Next(query.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrNotHeld), ShouldBeTrue)
		})
		Convey("Incomplete addresses fail before access", func() {
			query.Entity = ""
			So(tests.CollectSeq[core.Primitive](grid.Next(query.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrShape), ShouldBeTrue)
		})
		Convey("Writes cannot replace a member's value", func() {
			query.Actionable = data.ActionWrite
			So(tests.CollectSeq[core.Primitive](grid.Next(query.Next(nil))), ShouldBeEmpty)
			So(errors.Is(grid.Error(), core.ErrDomain), ShouldBeTrue)
			So(sequence.Read[float64](resident.Next(nil)), ShouldEqual, 0)
		})
	})
	Convey("A member's execution failure reaches the Grid", t, func() {
		member := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewGet[string, float64]("missing")}
		grid := store.NewGrid(map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": member})
		query := store.NewQuery[*geometry.Coordinate, map[string]float64](nil, data.ActionExecute)
		query.Entity, query.Address = "BTC/USD", member.Identity()
		for range grid.Next(query.Next(sequence.NewValue(map[string]float64{"present": 1}))) {
			t.Fatal("failed member produced output")
		}
		So(errors.Is(grid.Error(), core.ErrNotHeld), ShouldBeTrue)
	})
}

func BenchmarkGridNext(b *testing.B) {
	member := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(0, 0), Primitive: store.NewRetained(0.0)}
	grid := store.NewGrid(map[string]core.Identifiable[*geometry.Coordinate]{"BTC/USD": member})
	b.Run("Read", func(b *testing.B) {
		query := store.NewQuery[*geometry.Coordinate, float64](nil, data.ActionRead)
		query.Entity, query.Address = "BTC/USD", member.Identity()
		run := grid.Next(query.Next(nil))
		b.ReportAllocs()
		for b.Loop() {
			for range run {
			}
		}
	})
	b.Run("Execute", func(b *testing.B) {
		query := store.NewQuery[*geometry.Coordinate, float64](nil, data.ActionExecute, sequence.NewValue(1.0, 4.0, -2.0, 0.0))
		query.Entity, query.Address = "BTC/USD", member.Identity()
		run := grid.Next(query.Next(nil))
		b.ReportAllocs()
		for b.Loop() {
			for range run {
			}
		}
	})
	if err := grid.Error(); err != nil {
		b.Fatal(err)
	}
}

/*
orderedIndex exercises Grid with a non-geometric value identity, including zero.
*/
type orderedIndex int

func (index orderedIndex) Less(other orderedIndex) bool { return index < other }

func TestGridNextIdentities(t *testing.T) {
	Convey("Grid accepts non-geometric identities without special zero handling", t, func() {
		member := &tests.Member[orderedIndex]{Primitive: store.NewRetained(9.0), Address: 0}
		grid := store.NewGrid(map[string]core.Identifiable[orderedIndex]{"example": member})
		query := store.NewQuery[orderedIndex, float64](nil, data.ActionRead)
		query.Entity, query.Address = "example", 0
		So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, member)
		query.Actionable = data.ActionExecute
		So(tests.CollectSeq[float64](grid.Next(query.Next(sequence.NewValue(4.0, 7.0)))), ShouldResemble, []float64{4, 7})
		So(grid.Error(), ShouldBeNil)
	})
	Convey("A Coordinate can itself be a resident primitive", t, func() {
		coordinate := geometry.NewCoordinate(-2, 5)
		grid := store.NewGrid(map[string]core.Identifiable[*geometry.Coordinate]{"space": coordinate})
		query := store.NewQuery[*geometry.Coordinate, int](nil, data.ActionRead)
		query.Entity, query.Address = "space", geometry.NewCoordinate(-2, 5)
		So(sequence.Read[core.Primitive](grid.Next(query.Next(nil))), ShouldEqual, coordinate)
		query.Actionable = data.ActionExecute
		So(tests.CollectSeq[int](grid.Next(query.Next(nil))), ShouldResemble, []int{-2, 5})
		So(grid.Error(), ShouldBeNil)
	})
}

func TestGridNextNested(t *testing.T) {
	Convey("An owner's nested grid lookup cannot hide that owner's execution failure", t, func() {
		resident := store.NewGrid[*geometry.Coordinate]()
		peer := &tests.Member[*geometry.Coordinate]{Address: geometry.NewCoordinate(1, 0), Primitive: store.NewRetained(2.0)}
		read := store.NewQuery[*geometry.Coordinate, core.Primitive](peer, data.ActionRead)
		read.Entity = "BTC/USD"
		owner := &tests.Member[*geometry.Coordinate]{
			Address: geometry.NewCoordinate(0, 0),
			Primitive: nomagique.NewNumber(
				transport.NewOnce(nomagique.NewNumber(read, resident)),
				store.NewGet[string, float64]("missing"),
			),
		}
		for _, member := range []core.Identifiable[*geometry.Coordinate]{owner, peer} {
			identify := store.NewQuery[*geometry.Coordinate, core.Identifiable[*geometry.Coordinate]](member, data.ActionIdentify, sequence.NewValue(member))
			identify.Entity = "BTC/USD"
			for range resident.Next(identify.Next(nil)) {
			}
		}
		execute := store.NewQuery[*geometry.Coordinate, map[string]float64](owner, data.ActionExecute)
		execute.Entity = "BTC/USD"
		So(tests.CollectSeq[float64](resident.Next(execute.Next(sequence.NewValue(map[string]float64{"present": 1})))), ShouldBeEmpty)
		So(errors.Is(owner.Error(), core.ErrNotHeld), ShouldBeTrue)
		So(peer.Error(), ShouldBeNil)
		So(errors.Is(resident.Error(), core.ErrNotHeld), ShouldBeTrue)
	})
}
