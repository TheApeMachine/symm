package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestComplete(t *testing.T) {
	ctx := context.Background()

	Convey("Given which of four nodes are present", t, func() {
		client := graph.Complete_ServerToClient(graph.NewComplete())
		defer client.Release()

		edges := func(present []bool, reach ...bool) ([]int64, []int64, []int64) {
			So(client.Write(ctx, func(params graph.Complete_write_Params) error {
				params.SetReach(len(reach) > 0 && reach[0])

				flags, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for node, flag := range present {
					flags.Set(node, flag)
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			fromNodes, err := results.FromNodes()
			So(err, ShouldBeNil)
			toNodes, err := results.ToNodes()
			So(err, ShouldBeNil)
			pairs, err := results.Pairs()
			So(err, ShouldBeNil)

			from, to, named := []int64{}, []int64{}, []int64{}

			for edge := range pairs.Len() {
				from = append(from, fromNodes.At(edge))
				to = append(to, toNodes.At(edge))
				named = append(named, pairs.At(edge))
			}

			return from, to, named
		}

		Convey("Every pair of present nodes is one edge, addressed in the full matrix", func() {
			from, to, pairs := edges([]bool{true, false, true, true})
			So(from, ShouldResemble, []int64{0, 0, 2})
			So(to, ShouldResemble, []int64{2, 3, 3})
			So(pairs, ShouldResemble, []int64{2, 3, 11})

			Convey("And with reach, every present node is also joined to every absent one", func() {
				from, to, _ := edges([]bool{true, false, false, true}, true)
				So(from, ShouldResemble, []int64{0, 0, 0, 1, 2})
				So(to, ShouldResemble, []int64{1, 2, 3, 3, 3})
			})

			Convey("And the next evaluation starts clean", func() {
				from, _, _ := edges([]bool{true, false, false, false})
				So(from, ShouldBeEmpty)
			})
		})
	})
}
