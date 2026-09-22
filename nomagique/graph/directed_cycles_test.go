package graph_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/graph"
)

func TestDirectedCycles(t *testing.T) {
	Convey("Given a DirectedCycles server and client", t, func() {
		ctx := context.Background()
		server := graph.NewDirectedCycles(ctx)
		So(server, ShouldNotBeNil)

		client := graph.DirectedCycles_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params graph.DirectedCycles_write_Params) error {
				listFromNodes, err := params.NewFromNodes(3)

				if err != nil {
					return err
				}
				listFromNodes.Set(0, 0)
				listFromNodes.Set(1, 1)
				listFromNodes.Set(2, 2)
				listToNodes, err := params.NewToNodes(3)

				if err != nil {
					return err
				}
				listToNodes.Set(0, 1)
				listToNodes.Set(1, 2)
				listToNodes.Set(2, 0)
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
