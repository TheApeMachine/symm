package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSymbolCadenceTouch(t *testing.T) {
	Convey("Given a fresh symbolCadence", t, func() {
		cadence := &symbolCadence{}

		Convey("When touched for the first time", func() {
			mean := cadence.touch(time.Unix(10, 0))

			Convey("It should return zero mean and record the timestamp", func() {
				So(mean, ShouldEqual, 0)
				So(cadence.last, ShouldEqual, time.Unix(10, 0))
			})
		})

		Convey("When touched twice with a 2-second gap", func() {
			cadence.touch(time.Unix(10, 0))
			mean := cadence.touch(time.Unix(12, 0))

			Convey("It should compute the correct mean gap", func() {
				So(mean, ShouldEqual, 2*time.Second)
			})
		})

		Convey("When touched with a timestamp in the past", func() {
			cadence.touch(time.Unix(10, 0))
			mean := cadence.touch(time.Unix(8, 0))

			Convey("It should not update the mean", func() {
				So(mean, ShouldEqual, 0)
			})
		})
	})

	Convey("Given a nil symbolCadence", t, func() {
		var cadence *symbolCadence

		Convey("It should return zero without panicking", func() {
			So(cadence.touch(time.Unix(1, 0)), ShouldEqual, 0)
		})
	})
}

func TestCadenceBookTouch(t *testing.T) {
	Convey("Given an empty cadenceBook", t, func() {
		book := cadenceBook{}

		Convey("When touched for a new symbol", func() {
			mean := book.touch("SIM/USD", time.Unix(10, 0))

			Convey("It should return zero and initialize the symbol entry", func() {
				So(mean, ShouldEqual, 0)
				So(book.symbols["SIM/USD"], ShouldNotBeNil)
			})
		})

		Convey("When touched multiple times for the same symbol", func() {
			book.touch("SIM/USD", time.Unix(10, 0))
			mean := book.touch("SIM/USD", time.Unix(13, 0))

			Convey("It should compute the correct inter-cut mean", func() {
				So(mean, ShouldEqual, 3*time.Second)
			})
		})
	})
}

func TestDecayIdle(t *testing.T) {
	Convey("Given a graph with one edge that was not touched on this cut", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		edgeAt := time.Unix(90, 0).UTC()

		graph.strengthen(
			edgeAt, "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 1.0,
			[]string{"ignition"},
		)

		clear(graph.touched)
		mean := 5 * time.Second
		decayAt := edgeAt.Add(10 * time.Second)
		book := cadenceBook{}

		Convey("When decayIdle runs at a later timestamp", func() {
			book.decayIdle(graph, "SIM/USD", decayAt, mean)

			Convey("It should reduce the edge weight below original", func() {
				So(graph.Edges[0].Weight, ShouldBeLessThan, 1.0)
				So(graph.Edges[0].Weight, ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given a graph with a touched edge", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		at := time.Unix(100, 0).UTC()

		graph.strengthen(
			at, "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 1.0, nil,
		)

		book := cadenceBook{}
		mean := 5 * time.Second

		Convey("When decayIdle runs", func() {
			book.decayIdle(graph, "SIM/USD", at.Add(10*time.Second), mean)

			Convey("It should not decay the touched edge", func() {
				So(graph.Edges[0].Weight, ShouldEqual, 1.0)
			})
		})
	})

	Convey("Given a graph with a zero-weight edge after decay", t, func() {
		graph := NewGraph()
		graph.touched = map[edgeKey]struct{}{}
		edgeAt := time.Unix(10, 0).UTC()

		graph.strengthen(
			edgeAt, "SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			Supports, 0.001, nil,
		)

		clear(graph.touched)
		graph.Edges[0].Weight = 0
		book := cadenceBook{}

		Convey("When decayIdle runs", func() {
			book.decayIdle(graph, "SIM/USD", edgeAt.Add(time.Hour), time.Second)

			Convey("It should remove the dead edge from both slice and index", func() {
				So(len(graph.Edges), ShouldEqual, 0)
				So(len(graph.EdgeIndex), ShouldEqual, 0)
			})
		})
	})

	Convey("Given nil or zero arguments", t, func() {
		book := cadenceBook{}

		Convey("It should not panic with nil graph", func() {
			So(func() {
				book.decayIdle(nil, "SIM/USD", time.Unix(1, 0), time.Second)
			}, ShouldNotPanic)
		})

		Convey("It should not panic with zero mean", func() {
			graph := NewGraph()
			So(func() {
				book.decayIdle(graph, "SIM/USD", time.Unix(1, 0), 0)
			}, ShouldNotPanic)
		})
	})
}

func BenchmarkDecayIdle(b *testing.B) {
	graph := NewGraph()
	graph.touched = map[edgeKey]struct{}{}
	base := time.Unix(100, 0).UTC()
	categoryTypes := []types.CategoryType{
		types.VerticalIgnition, types.OrganicTrend,
		types.CoiledCompression, types.Exhaustion,
	}

	for left := range categoryTypes {
		for right := left + 1; right < len(categoryTypes); right++ {
			graph.strengthen(
				base, "SIM/USD",
				categoryTypes[left], categoryTypes[right],
				Supports, 1.0, nil,
			)
		}
	}

	book := cadenceBook{}
	mean := 5 * time.Second

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		clear(graph.touched)
		book.decayIdle(graph, "SIM/USD", base.Add(time.Duration(index)*time.Second), mean)
	}
}
