package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestPageRank(t *testing.T) {
	Convey("Given a PageRank server and client", t, func() {
		ctx := context.Background()
		server := graph.NewPageRank(ctx)
		So(server, ShouldNotBeNil)

		client := graph.PageRank_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params graph.PageRank_write_Params) error {
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
				params.SetDamping(0.85)
				params.SetTol(1e-06)
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
