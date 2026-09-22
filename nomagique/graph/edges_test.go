package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestEdgesWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given a square matrix of relationships", t, func() {
		client := graph.Edges_ServerToClient(graph.NewEdges(ctx))
		defer client.Release()

		// Three quantities: the first two agree strongly, the third with
		// neither. The second pair is strongly inverse.
		matrix := []float64{
			1.0, 0.9, 0.1,
			0.9, 1.0, -0.8,
			0.1, -0.8, 1.0,
		}

		read := func(threshold float64, signed bool) (
			from, to []int64, weights []float64, kept float64,
		) {
			err := client.Write(ctx, func(params graph.Edges_write_Params) error {
				params.SetDim(3)
				params.SetThreshold(threshold)
				params.SetSigned(signed)

				held, err := params.NewMatrix(int32(len(matrix)))

				if err != nil {
					return err
				}

				for index, value := range matrix {
					held.Set(index, value)
				}

				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			fromNodes, err := results.FromNodes()
			So(err, ShouldBeNil)
			toNodes, err := results.ToNodes()
			So(err, ShouldBeNil)
			held, err := results.Weights()
			So(err, ShouldBeNil)

			for index := range fromNodes.Len() {
				from = append(from, fromNodes.At(index))
				to = append(to, toNodes.At(index))
				weights = append(weights, held.At(index))
			}

			return from, to, weights, results.Kept()
		}

		Convey("When the cut keeps only the strongest relationships", func() {
			from, to, weights, kept := read(0.75, false)

			// An inverse relationship is a relationship, so the strongly
			// opposed pair is kept alongside the strongly agreeing one.
			Convey("Then a pair is one edge, taken from the upper triangle", func() {
				So(from, ShouldResemble, []int64{0, 1})
				So(to, ShouldResemble, []int64{1, 2})
				So(weights, ShouldResemble, []float64{0.9, 0.8})
				So(kept, ShouldAlmostEqual, 2.0/3.0, 0.0001)
			})
		})

		Convey("When the cut reads strength as signed", func() {
			from, _, _, _ := read(0.75, true)

			Convey("Then only the pair that agrees survives", func() {
				So(from, ShouldResemble, []int64{0})
			})
		})

		Convey("When nothing reaches the cut", func() {
			from, _, _, kept := read(0.99, false)

			Convey("Then the graph is empty and says so", func() {
				So(from, ShouldBeEmpty)
				So(kept, ShouldEqual, 0)
			})
		})

		Convey("When the matrix is not the square it claims", func() {
			err := client.Write(ctx, func(params graph.Edges_write_Params) error {
				params.SetDim(4)

				held, err := params.NewMatrix(9)

				if err != nil {
					return err
				}

				held.Set(0, 1)
				return nil
			})
			So(err, ShouldBeNil)

			Convey("Then it refuses rather than reading past the end", func() {
				So(client.WaitStreaming(), ShouldNotBeNil)
			})
		})
	})
}
