package runtime_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func bindGroup(ctx context.Context, consumers ...runtime.Consumer) (runtime.Group, error) {
	group := runtime.Group_ServerToClient(runtime.NewGroup(ctx))
	err := group.Write(ctx, func(params runtime.Group_write_Params) error {
		members, err := params.NewConsumers(int32(len(consumers)))

		if err != nil {
			return err
		}

		for index, consumer := range consumers {
			if err := members.Set(index, consumer.AddRef()); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		err = group.WaitStreaming()
	}

	if err != nil {
		group.Release()
		return runtime.Group{}, err
	}
	return group, nil
}

func TestGroupWrite(t *testing.T) {
	Convey("Given a group of two consumer capabilities", t, func() {
		ctx := context.Background()
		target := runtime.StageNode_ServerToClient(&recordingStage{})
		defer target.Release()
		first, err := bindConsumer(ctx, target)
		So(err, ShouldBeNil)
		defer first.Release()
		second, err := bindConsumer(ctx, target)
		So(err, ShouldBeNil)
		defer second.Release()
		group, err := bindGroup(ctx, first, second)
		So(err, ShouldBeNil)
		defer group.Release()
		future, release := group.Members(ctx, nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		members, err := result.Consumers()
		So(err, ShouldBeNil)
		So(members.Len(), ShouldEqual, 2)
		actual, err := members.At(0)
		So(err, ShouldBeNil)
		So(actual.IsSame(first), ShouldBeTrue)
		actual, err = members.At(1)
		So(err, ShouldBeNil)
		So(actual.IsSame(second), ShouldBeTrue)

		Convey("Membership cannot be replaced after configuration", func() {
			So(group.Write(ctx, func(params runtime.Group_write_Params) error {
				changed, err := params.NewConsumers(1)

				if err != nil {
					return err
				}
				return changed.Set(0, second.AddRef())
			}), ShouldBeNil)
			So(group.WaitStreaming(), ShouldNotBeNil)
		})
	})
	Convey("An empty group is rejected", t, func() {
		_, err := bindGroup(context.Background())
		So(err, ShouldNotBeNil)
	})
}
