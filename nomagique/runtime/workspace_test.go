package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* workspaceFixture owns every capability reference created for a test pipeline. */
type workspaceFixture struct {
	client    runtime.Workspace
	consumers []runtime.Consumer
	groups    []runtime.Group
	targets   []runtime.StageNode
	sources   bool
}

func newWorkspaceFixture(ctx context.Context, capacity uint32, stages ...[]*recordingStage) (*workspaceFixture, error) {
	fixture := &workspaceFixture{client: runtime.Workspace_ServerToClient(runtime.NewWorkspace(ctx))}
	if len(stages) > 0 && len(stages[0]) > 0 {
		fixture.sources = stages[0][0].source != nil
	}

	for _, stagesInGroup := range stages {
		members := []runtime.Consumer{}

		for _, stage := range stagesInGroup {
			target := runtime.StageNode_ServerToClient(stage)
			fixture.targets = append(fixture.targets, target)
			consumer, err := bindConsumer(ctx, target)

			if err != nil {
				fixture.release()
				return nil, err
			}
			fixture.consumers = append(fixture.consumers, consumer)
			members = append(members, consumer)
		}
		group, err := bindGroup(ctx, members...)

		if err != nil {
			fixture.release()
			return nil, err
		}
		fixture.groups = append(fixture.groups, group)
	}
	if err := fixture.configure(ctx, capacity, true); err != nil {
		fixture.release()
		return nil, err
	}
	return fixture, nil
}

func (fixture *workspaceFixture) configure(ctx context.Context, capacity uint32, admit bool) error {
	err := fixture.client.Write(ctx, func(params runtime.Workspace_write_Params) error {
		params.SetCapacity(capacity)
		params.SetWriters(1)
		params.SetEpoch(77)
		params.SetAdmit(admit)
		params.SetAdvance(fixture.sources)
		groups, err := params.NewGroups(int32(len(fixture.groups)))

		if err != nil {
			return err
		}

		for index, group := range fixture.groups {
			if err := groups.Set(index, group.AddRef()); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		err = fixture.client.WaitStreaming()
	}

	return err
}

func (fixture *workspaceFixture) release() {
	fixture.client.Release()

	for _, group := range fixture.groups {
		group.Release()
	}

	for _, consumer := range fixture.consumers {
		consumer.Release()
	}

	for _, target := range fixture.targets {
		target.Release()
	}
}

func (fixture *workspaceFixture) publish(ctx context.Context, payloads ...string) error {
	err := fixture.client.Write(ctx, func(params runtime.Workspace_write_Params) error {
		data, err := params.NewData(int32(len(payloads)))

		if err != nil {
			return err
		}

		for index, payload := range payloads {
			if err := data.Set(index, []byte(payload)); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	return fixture.client.WaitStreaming()
}

func (fixture *workspaceFixture) flush(ctx context.Context) error {
	future, release := fixture.client.Flush(ctx, nil)
	defer release()
	_, err := future.Struct()
	return err
}

func TestWorkspaceWrite(t *testing.T) {
	Convey("Given parallel consumers followed by a dependent group", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		first, second := &recordingStage{}, &recordingStage{}
		downstream := &recordingStage{before: func(observation recordedObservation) error {
			if len(first.snapshot()) <= int(observation.sequence) || len(second.snapshot()) <= int(observation.sequence) {
				return errors.New("later group overtook an upstream consumer")
			}
			return nil
		}}
		fixture, err := newWorkspaceFixture(ctx, 8, []*recordingStage{first, second}, []*recordingStage{downstream})
		So(err, ShouldBeNil)
		defer fixture.release()

		Convey("A configured ring rejects attempts to change its capacity", func() {
			So(fixture.configure(ctx, 16, true), ShouldNotBeNil)
		})

		Convey("All feeds retain ordered stamps through repeated ring wraparound", func() {
			expected := []recordedObservation{}

			for sequence := range 64 {
				payload := fmt.Sprintf("feed-%d", sequence%3)
				So(fixture.publish(ctx, payload), ShouldBeNil)
				expected = append(expected, recordedObservation{77, int64(sequence), payload})
			}
			So(fixture.flush(ctx), ShouldBeNil)
			So(first.snapshot(), ShouldResemble, expected)
			So(second.snapshot(), ShouldResemble, expected)
			So(downstream.snapshot(), ShouldResemble, expected)
			future, release := fixture.client.Done(ctx, nil)
			defer release()
			progress, err := future.Struct()
			So(err, ShouldBeNil)
			So(progress.Published(), ShouldEqual, 64)
			So(progress.Completed(), ShouldEqual, 64)
			So(progress.Pending(), ShouldEqual, 0)
		})
	})

	Convey("Given one blocked consumer alongside a completed consumer", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gate := make(chan struct{})
		blocked := &recordingStage{entered: make(chan struct{}, 1), gate: gate}
		downstream := &recordingStage{}
		fixture, err := newWorkspaceFixture(ctx, 8, []*recordingStage{{}, blocked}, []*recordingStage{downstream})
		So(err, ShouldBeNil)
		defer fixture.release()
		So(fixture.publish(ctx, "ticker", "trade"), ShouldBeNil)
		select {
		case <-blocked.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		So(downstream.snapshot(), ShouldBeEmpty)
		close(gate)
		So(fixture.flush(ctx), ShouldBeNil)
		So(downstream.snapshot(), ShouldResemble, []recordedObservation{{77, 0, "ticker"}, {77, 1, "trade"}})
	})

	Convey("A failed consumer prevents downstream acknowledgement", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		downstream := &recordingStage{}
		fixture, err := newWorkspaceFixture(ctx, 8, []*recordingStage{{failure: errors.New("fixture failure")}}, []*recordingStage{downstream})
		So(err, ShouldBeNil)
		defer fixture.release()
		So(fixture.publish(ctx, "ticker"), ShouldBeNil)
		So(fixture.flush(ctx), ShouldNotBeNil)
		So(downstream.snapshot(), ShouldBeEmpty)
	})

	Convey("Invalid ring configuration is rejected through the node protocol", t, func() {
		_, err := newWorkspaceFixture(context.Background(), 3, []*recordingStage{{}})
		So(err, ShouldNotBeNil)
		_, err = newWorkspaceFixture(context.Background(), 8)
		So(err, ShouldNotBeNil)
	})
}

/* BenchmarkWorkspaceWrite includes completion of all three consumer calls. */
func BenchmarkWorkspaceWrite(b *testing.B) {
	ctx := context.Background()
	fixture, err := newWorkspaceFixture(ctx, 1024, []*recordingStage{{}, {}}, []*recordingStage{{}})

	if err != nil {
		b.Fatal(err)
	}
	defer fixture.release()
	b.ReportAllocs()

	for b.Loop() {
		if err := fixture.publish(ctx, `{"channel":"ticker","symbol":"BTC/USD","last":65000}`); err != nil {
			b.Fatal(err)
		}

		if err := fixture.flush(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func TestWorkspaceStep(t *testing.T) {
	Convey("A workspace can be wired as another Consumer node's stage", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		leaf := &recordingStage{}
		inner, err := newWorkspaceFixture(ctx, 8, []*recordingStage{leaf})
		So(err, ShouldBeNil)
		defer inner.release()
		consumer, err := bindConsumer(ctx, runtime.StageNode(inner.client))
		So(err, ShouldBeNil)
		defer consumer.Release()
		So(stepConsumer(ctx, consumer, 91, 120, "nested-ticker"), ShouldBeNil)
		So(stepConsumer(ctx, consumer, 91, 121, "nested-trade"), ShouldBeNil)
		So(leaf.snapshot(), ShouldResemble, []recordedObservation{{91, 120, "nested-ticker"}, {91, 121, "nested-trade"}})
	})
}

/* TestWorkspaceStepResults preserves parent inputs and inner results across nested ring wraparound. */
func TestWorkspaceStepResults(t *testing.T) {
	Convey("A nested workspace preserves typed observation results", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		inner, err := newWorkspaceFixture(ctx, 2, []*recordingStage{{echo: true}})
		So(err, ShouldBeNil)
		defer inner.release()
		for sequence := range 64 {
			future, release := inner.client.Step(ctx, func(params runtime.StageNode_step_Params) error {
				params.SetEpoch(91)
				params.SetSequence(int64(sequence))
				upstream, err := params.NewUpstream(1)
				if err != nil {
					return err
				}
				result := upstream.At(0)
				if err := result.SetNode("observation"); err != nil {
					return err
				}
				value, err := capnp.NewText(params.Segment(), fmt.Sprintf("value-%d", sequence))
				if err != nil {
					return err
				}
				return result.SetValue(value.ToPtr())
			})
			completed, err := future.Struct()
			So(err, ShouldBeNil)
			outputs, err := completed.Outputs()
			So(err, ShouldBeNil)
			So(outputs.Len(), ShouldEqual, 1)
			So(outputs.At(0).Epoch(), ShouldEqual, 91)
			So(outputs.At(0).Sequence(), ShouldEqual, sequence)
			value, err := outputs.At(0).Value()
			So(err, ShouldBeNil)
			So(value.Text(), ShouldEqual, fmt.Sprintf("value-%d", sequence))
			release()
		}
	})
}
