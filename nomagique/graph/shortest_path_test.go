package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestShortestPath(t *testing.T) {
	Convey("Given a ShortestPath server and client", t, func() {
		ctx := context.Background()
		server := graph.NewShortestPath(ctx)
		So(server, ShouldNotBeNil)

		client := graph.ShortestPath_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params graph.ShortestPath_write_Params) error {
				listFromNodes, err := params.NewFromNodes(2)

				if err != nil {
					return err
				}
				listFromNodes.Set(0, 0)
				listFromNodes.Set(1, 1)
				listToNodes, err := params.NewToNodes(2)

				if err != nil {
					return err
				}
				listToNodes.Set(0, 1)
				listToNodes.Set(1, 2)
				listWeights, err := params.NewWeights(2)

				if err != nil {
					return err
				}
				listWeights.Set(0, 2.0)
				listWeights.Set(1, 3.0)
				params.SetSource(0)
				params.SetTarget(2)
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
