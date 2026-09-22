package compiler

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"encoding/json"
	"github.com/theapemachine/symm/nomagique/compiler/testdata/projectionfixture"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorkbenchRunnerRun(t *testing.T) {
	Convey("Given a preview with no completed excursion", t, func() {
		response, err := NewWorkbenchRunner().Run(context.Background(), []byte(`{"nodes":{"excursion":{"id":"excursion","type":"temporal.Excursion","inputData":{"value":{"value":100}}}}}`))
		So(err, ShouldBeNil)
		run := response.(RunResponse)
		So(run.OK, ShouldBeTrue)
		result := run.Results["excursion"]
		So(result, ShouldContainKey, "none")
		So(result, ShouldNotContainKey, "move.anchor")
		So(result, ShouldNotContainKey, "move.ignition")
		So(result, ShouldNotContainKey, "move.extremum")
	})
}

/*
projectionOutput is a real RPC producer with deterministic wire fixtures.
*/
type projectionOutput struct{}

func (*projectionOutput) Write(context.Context, projectionfixture.Output_write) error { return nil }

func (*projectionOutput) Done(ctx context.Context, call projectionfixture.Output_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	value, err := result.NewValue()
	if err != nil {
		return err
	}
	if err := value.SetLabel("observed"); err != nil {
		return err
	}
	value.SetAmount(-12.5)
	points, err := result.NewPoints(3)
	if err != nil {
		return err
	}
	points.Set(0, 100)
	points.Set(1, 200)
	points.Set(2, 100)
	return nil
}

func TestWorkbenchRunnerImplRun(t *testing.T) {
	Convey("Given a real RPC producer connected to a graph UI component", t, func() {
		registry := NewRegistry()
		registry.Register("fixture.Output", Factory{InterfaceID: projectionfixture.Output_TypeID, New: func(context.Context, []byte) (capnp.Client, error) {
			return capnp.Client(projectionfixture.Output_ServerToClient(&projectionOutput{})), nil
		}})
		runner := &WorkbenchRunnerImpl{reg: registry}
		response, err := runner.Run(context.Background(), []byte(`{"nodes":{
   "source":{"id":"source","type":"fixture.Output","connections":{"outputs":{"points":[{"nodeId":"spark","portName":"points"}]}}},
   "spark":{"id":"spark","type":"ui.Sparkline","connections":{"inputs":{"points":[{"nodeId":"source","portName":"points"}]}}}
  }}`))
		So(err, ShouldBeNil)
		result := response.(RunResponse)
		So(result.OK, ShouldBeTrue)
		So(result.Results["source"]["points"], ShouldResemble, []any{float64(100), float64(200), float64(100)})
		value := result.Results["source"]["value"].(map[string]any)
		So(value["label"], ShouldEqual, "observed")
		So(value["amount"], ShouldEqual, -12.5)
		encoded, err := json.Marshal(result)
		So(err, ShouldBeNil)
		So(string(encoded), ShouldContainSubstring, `"points":[100,200,100]`)
		So(string(encoded), ShouldNotContainSubstring, "<list:")
		So(string(encoded), ShouldNotContainSubstring, "<struct>")
	})
}
