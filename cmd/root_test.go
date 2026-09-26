package cmd_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestExecute(t *testing.T) {
	Convey("Given the system pipeline definition", t, func() {
		graph, err := compiler.DefaultRepository().Load("system")
		So(err, ShouldBeNil)
		// Exercise the shipping HTTP/query node lifecycle without admitting
		// external market feeds or opening the user's durable model files.
		for id := range graph.Nodes {
			if id != "server" && id != "inspection" {
				delete(graph.Nodes, id)
			}
		}
		for id, node := range graph.Nodes {
			for _, ports := range []map[string][]compiler.ConnectionTarget{node.Connections.Inputs, node.Connections.Outputs} {
				for port, targets := range ports {
					kept := []compiler.ConnectionTarget{}
					for _, target := range targets {
						if _, found := graph.Nodes[target.NodeID]; found {
							kept = append(kept, target)
						}
					}
					if len(kept) == 0 {
						delete(ports, port)
						continue
					}
					ports[port] = kept
				}
			}
			graph.Nodes[id] = node
		}
		server := graph.Nodes["server"]
		server.InputData["address"] = json.RawMessage(`"127.0.0.1:0"`)
		graph.Nodes["server"] = server
		pipeline, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		So(pipeline, ShouldNotBeNil)
		defer pipeline.Release()

		Convey("It executes gracefully on context cancellation", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- pipeline.Start(ctx)
			}()

			select {
			case err := <-done:
				So(err, ShouldBeNil)
			case <-time.After(500 * time.Millisecond):
				t.Fatal("system graph failed to stop after cancellation")
			}
		})
	})
}
