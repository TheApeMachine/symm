package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestMinimumSpanningTree(t *testing.T) {
	Convey("Given a MinimumSpanningTree server and client", t, func() {
		ctx := context.Background()
		server := graph.NewMinimumSpanningTree(ctx)
		So(server, ShouldNotBeNil)

		client := graph.MinimumSpanningTree_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params graph.MinimumSpanningTree_write_Params) error {
				listFromNodes, err := params.NewFromNodes(3)

				if err != nil {
					return err
				}
				listFromNodes.Set(0, 0)
				listFromNodes.Set(1, 1)
				listFromNodes.Set(2, 0)
				listToNodes, err := params.NewToNodes(3)

				if err != nil {
					return err
				}
				listToNodes.Set(0, 1)
				listToNodes.Set(1, 2)
				listToNodes.Set(2, 2)
				listWeights, err := params.NewWeights(3)

				if err != nil {
					return err
				}
				listWeights.Set(0, 1.0)
				listWeights.Set(1, 2.0)
				listWeights.Set(2, 4.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
