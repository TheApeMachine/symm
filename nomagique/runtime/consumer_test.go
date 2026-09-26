package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* recordedObservation preserves the exact stamp and payload received by a stage. */
type recordedObservation struct {
	epoch, sequence int64
	payload         string
}

/* recordingStage is the shared RPC fixture for consumer, group and workspace tests. */
type recordingStage struct {
	mutex        sync.Mutex
	observations []recordedObservation
	entered      chan struct{}
	gate         <-chan struct{}
	failure      error
	echo         bool
	before       func(recordedObservation) error
}

func (stage *recordingStage) Step(ctx context.Context, call runtime.StageNode_step) error {
	payload, err := call.Args().Data()

	if err != nil {
		return err
	}
	observation := recordedObservation{call.Args().Epoch(), call.Args().Sequence(), string(payload)}

	if stage.entered != nil {
		stage.entered <- struct{}{}
	}

	if stage.gate != nil {
		select {
		case <-stage.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if stage.failure != nil {
		return stage.failure
	}

	if stage.before != nil {
		if err := stage.before(observation); err != nil {
			return err
		}
	}
	stage.mutex.Lock()
	stage.observations = append(stage.observations, observation)
	stage.mutex.Unlock()
	if stage.echo {
		upstream, err := call.Args().Upstream()
		if err != nil {
			return err
		}
		results, err := call.AllocResults()
		if err != nil {
			return err
		}
		return results.SetOutputs(upstream)
	}
	return nil
}

func (stage *recordingStage) snapshot() []recordedObservation {
	stage.mutex.Lock()
	defer stage.mutex.Unlock()
	return append([]recordedObservation(nil), stage.observations...)
}

func bindConsumer(ctx context.Context, target runtime.StageNode) (runtime.Consumer, error) {
	consumer := runtime.Consumer_ServerToClient(runtime.NewConsumer(ctx))
	err := consumer.Write(ctx, func(params runtime.Consumer_write_Params) error { return params.SetTarget(target.AddRef()) })

	if err == nil {
		err = consumer.WaitStreaming()
	}

	if err != nil {
		consumer.Release()
		return runtime.Consumer{}, err
	}
	return consumer, nil
}

func stepConsumer(ctx context.Context, consumer runtime.Consumer, epoch, sequence int64, payload string) error {
	future, release := consumer.Step(ctx, func(params runtime.StageNode_step_Params) error {
		params.SetEpoch(epoch)
		params.SetSequence(sequence)
		return params.SetData([]byte(payload))
	})
	defer release()
	_, err := future.Struct()
	return err
}

func TestConsumerStep(t *testing.T) {
	Convey("Given a consumer wired to a real stage capability", t, func() {
		ctx := context.Background()
		stage := &recordingStage{}
		target := runtime.StageNode_ServerToClient(stage)
		defer target.Release()
		consumer, err := bindConsumer(ctx, target)
		So(err, ShouldBeNil)
		defer consumer.Release()

		Convey("It preserves stamps and acknowledges only successful work", func() {
			So(stepConsumer(ctx, consumer, 77, 10, "ticker"), ShouldBeNil)
			So(stepConsumer(ctx, consumer, 77, 11, "trade"), ShouldBeNil)
			So(stage.snapshot(), ShouldResemble, []recordedObservation{{77, 10, "ticker"}, {77, 11, "trade"}})
			future, release := consumer.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Completed(), ShouldEqual, 2)
			So(result.Epoch(), ShouldEqual, 77)
			So(result.Sequence(), ShouldEqual, 11)
		})

		Convey("A failed stage remains failed and cannot acknowledge later observations", func() {
			stage.failure = errors.New("fixture stage failure")
			So(stepConsumer(ctx, consumer, 77, 10, "failed"), ShouldNotBeNil)
			So(stepConsumer(ctx, consumer, 77, 11, "later"), ShouldNotBeNil)
			So(stage.snapshot(), ShouldBeEmpty)
			future, release := consumer.Done(ctx, nil)
			defer release()
			_, err := future.Struct()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestConsumerWrite(t *testing.T) {
	Convey("Given an unconfigured consumer", t, func() {
		ctx := context.Background()
		consumer := runtime.Consumer_ServerToClient(runtime.NewConsumer(ctx))
		defer consumer.Release()
		So(stepConsumer(ctx, consumer, 1, 0, "early"), ShouldNotBeNil)
		So(consumer.Write(ctx, nil), ShouldBeNil)
		So(consumer.WaitStreaming(), ShouldNotBeNil)
	})
}

func TestConsumerWriteConfiguration(t *testing.T) {
	Convey("A configured Consumer cannot silently keep an outdated binding", t, func() {
		for _, change := range []string{"name", "entry", "bindings", "outputs"} {
			ctx := context.Background()
			target := runtime.StageNode_ServerToClient(&recordingStage{})
			defer target.Release()
			consumer, err := bindConsumer(ctx, target)
			So(err, ShouldBeNil)
			defer consumer.Release()
			So(consumer.Write(ctx, func(params runtime.Consumer_write_Params) error {
				if err := params.SetTarget(target.AddRef()); err != nil {
					return err
				}

				switch change {
				case "name":
					return params.SetName("changed")
				case "entry":
					return params.SetEntry("changed.data")
				case "bindings":
					return params.SetBindings("[]")
				case "outputs":
					outputs, err := params.NewOutputs(1)

					if err != nil {
						return err
					}
					return outputs.Set(0, "changed")
				}
				return nil
			}), ShouldBeNil)
			So(consumer.WaitStreaming(), ShouldNotBeNil)
		}
	})
}

func (stage *recordingStage) Fence(ctx context.Context, call runtime.StageNode_fence) error {
	return stage.failure
}
