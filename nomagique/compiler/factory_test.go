package compiler

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/* TestStageFactoryCreate proves replicas own independent state through real graph capabilities. */
func TestStageFactoryCreate(t *testing.T) {
	Convey("A graph factory creates independent authored state owners", t, func() {
		repository := NewRepository()
		So(repository.Save("calculation", []byte(`{"id":"reading","nodes":{"read":{"id":"read","type":"data.Extract","inputData":{"path":"value"},"connections":{"outputs":{"out":[{"nodeId":"mean","portName":"value"}]}}},"mean":{"id":"mean","type":"statistic.Mean","connections":{"inputs":{"value":[{"nodeId":"read","portName":"out"}]}}}}}`)), ShouldBeNil)
		So(repository.Save("reading", []byte(`{"id":"reading","nodes":{"calculate":{"id":"calculate","type":"definition:calculation"}}}`)), ShouldBeNil)
		program, err := CompileJSON([]byte(`{"id":"factory","nodes":{"factory":{"id":"factory","type":"factory:reading"}}}`), nil, repository)
		So(err, ShouldBeNil)
		defer program.Release()
		// A later editor save must not change the recipe of an already compiled factory.
		So(repository.Save("calculation", []byte(`{"id":"changed","nodes":{"invalid":{"id":"invalid","type":"missing.Operation"}}}`)), ShouldBeNil)
		factory := runtime.StageFactory(program.Nodes[program.NodeMap["factory"]].Client)
		children := make([]runtime.StageNode, 2)
		for index := range children {
			future, release := factory.Create(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			children[index] = result.Stage().AddRef()
			defer children[index].Release()
			release()
		}
		read := func(child runtime.StageNode, payload string) float64 {
			future, release := child.Step(context.Background(), func(args runtime.StageNode_step_Params) error {
				if err := args.SetEntry("calculate__read.data"); err != nil {
					return err
				}
				if err := args.SetData([]byte(payload)); err != nil {
					return err
				}
				outputs, err := args.NewOutputs(1)
				if err != nil {
					return err
				}
				return outputs.Set(0, "calculate__mean")
			})
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := result.Outputs()
			So(err, ShouldBeNil)
			So(outputs.Len(), ShouldEqual, 1)
			value, err := outputs.At(0).Value()
			So(err, ShouldBeNil)
			return statistic.Mean_done_Results(value.Struct()).Out()
		}
		So(read(children[0], `{"value":1}`), ShouldEqual, 1)
		So(read(children[1], `{"value":100}`), ShouldEqual, 100)
		So(read(children[0], `{"value":3}`), ShouldEqual, 2)
	})
}
