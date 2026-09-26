package compiler

import (
	"context"
	"fmt"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
)

/* TestStageFactoryStep exercises interleaved markets over the native stage protocol. */
func TestStageFactoryStep(t *testing.T) {
	Convey("One authored graph keeps independent market state", t, func() {
		factory := partitionFixture(t)
		defer factory.Release()
		Convey("Alternating markets resume their own history", func() {
			for sequence, sample := range []struct {
				symbol          string
				value, expected float64
			}{
				{"BTC/USD", 1, 1}, {"ETH/USD", 100, 100}, {"BTC/USD", 3, 2}, {"ETH/USD", 200, 150}, {"BTC/USD", 8, 4},
			} {
				value, err := partitionStep(factory, sample.symbol, int64(sequence), sample.value, "mean")
				So(err, ShouldBeNil)
				outputs, err := value.Outputs()
				So(err, ShouldBeNil)
				So(outputs.Len(), ShouldEqual, 1)
				pointer, err := outputs.At(0).Value()
				So(err, ShouldBeNil)
				So(statistic.Mean_done_Results(pointer.Struct()).Out(), ShouldEqual, sample.expected)
				value.Message().Release()
			}
			future, release := factory.Done(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Partitions(), ShouldEqual, 2)
		})
		Convey("A missing key is rejected before a graph is created", func() {
			_, err := partitionStep(factory, "", 0, 1, "mean")
			So(err, ShouldNotBeNil)
			future, release := factory.Done(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Partitions(), ShouldEqual, 0)
		})
		Convey("A quiet source does not create a market or advance its history", func() {
			future, release := factory.Step(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := result.Outputs()
			So(err, ShouldBeNil)
			So(outputs.Len(), ShouldEqual, 0)
			release()
			progress, release := factory.Done(context.Background(), nil)
			defer release()
			done, err := progress.Struct()
			So(err, ShouldBeNil)
			So(done.Partitions(), ShouldEqual, 0)
		})
	})
}

/* partitionFixture owns the authored child recipe used by partition tests and benchmarks. */
func partitionFixture(t testing.TB) runtime.StageFactory {
	t.Helper()
	repository := NewRepository()
	document := `{"id":"reading","nodes":{"read":{"id":"read","type":"data.Extract","inputData":{"path":"value"},"connections":{"outputs":{"out":[{"nodeId":"mean","portName":"value"}]}}},"mean":{"id":"mean","type":"statistic.Mean","connections":{"inputs":{"value":[{"nodeId":"read","portName":"out"}]}}}}}`

	if err := repository.Save("reading", []byte(document)); err != nil {
		t.Fatal(err)
	}
	program, err := CompileJSON([]byte(`{"id":"partition","nodes":{"factory":{"id":"factory","type":"factory:reading","inputData":{"producer":"projection","node":"grid","field":"scope"}}}}`), nil, repository)

	if err != nil {
		t.Fatal(err)
	}
	defer program.Release()

	if err := program.Execute(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return runtime.StageFactory(program.Nodes[program.NodeMap["factory"]].Client).AddRef()
}

/* partitionStep supplies an actual schema-backed projection result and retains its reply. */
func partitionStep(factory runtime.StageFactory, symbol string, sequence int64, value float64, output string) (runtime.Completion, error) {
	future, release := factory.Step(context.Background(), func(args runtime.StageNode_step_Params) error {
		args.SetEpoch(99)
		args.SetSequence(sequence)

		if err := args.SetEntry("read.data"); err != nil {
			return err
		}

		if err := args.SetData([]byte(fmt.Sprintf(`{"value":%g}`, value))); err != nil {
			return err
		}
		selected, err := args.NewOutputs(1)

		if err != nil {
			return err
		}

		if err := selected.Set(0, output); err != nil {
			return err
		}
		upstream, err := args.NewUpstream(1)

		if err != nil {
			return err
		}
		result := upstream.At(0)
		result.SetEpoch(99)
		result.SetSequence(sequence)
		result.SetInterfaceId(store.Grid_TypeID)

		if err := result.SetProducer("projection"); err != nil {
			return err
		}

		if err := result.SetNode("grid"); err != nil {
			return err
		}
		grid, err := store.NewGrid_done_Results(args.Segment())

		if err != nil {
			return err
		}

		if err := grid.SetScope(symbol); err != nil {
			return err
		}
		return result.SetValue(capnp.Struct(grid).ToPtr())
	})
	defer release()
	result, err := future.Struct()

	if err != nil {
		return runtime.Completion{}, err
	}
	return result.Clone()
}

/* BenchmarkStageFactoryStep includes native partition routing and the actual authored mean. */
func BenchmarkStageFactoryStep(b *testing.B) {
	factory := partitionFixture(b)
	defer factory.Release()
	for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
		result, err := partitionStep(factory, symbol, 0, 1, "mean")

		if err != nil {
			b.Fatal(err)
		}
		result.Message().Release()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		symbol := "BTC/USD"

		if index%2 == 1 {
			symbol = "ETH/USD"
		}
		result, err := partitionStep(factory, symbol, int64(index+1), float64(index+1), "mean")

		if err != nil {
			b.Fatal(err)
		}
		result.Message().Release()
	}
}
