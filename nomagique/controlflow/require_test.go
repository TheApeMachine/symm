package controlflow_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/controlflow"
)

func TestRequireWrite(t *testing.T) {
	Convey("Given a precondition declared in the graph", t, func() {
		client := controlflow.Require_ServerToClient(controlflow.NewRequire())
		defer client.Release()

		Convey("Accepted data is emitted unchanged and then cleared", func() {
			So(client.Write(context.Background(), func(params controlflow.Require_write_Params) error {
				params.SetTest(true)

				if err := params.SetReason("ordered observations are required"); err != nil {
					return err
				}
				return params.SetData([]byte(`{"sequence":9007199254740993}`))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			payload, err := result.Out()
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, `{"sequence":9007199254740993}`)
			release()
			future, release = client.Done(context.Background(), nil)
			result, err = future.Struct()
			So(err, ShouldBeNil)
			payload, err = result.Out()
			So(err, ShouldBeNil)
			So(payload, ShouldBeEmpty)
			release()
		})

		Convey("A false predicate reports its declared reason", func() {
			So(client.Write(context.Background(), func(params controlflow.Require_write_Params) error {
				return params.SetReason("out-of-order observation")
			}), ShouldBeNil)
			err := client.WaitStreaming()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "out-of-order observation")
		})

		Convey("An unnamed precondition cannot silently discard data", func() {
			So(client.Write(context.Background(), func(params controlflow.Require_write_Params) error {
				params.SetTest(true)
				return nil
			}), ShouldBeNil)
			err := client.WaitStreaming()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "precondition reason is required")
		})
	})
}

func BenchmarkRequireWrite(b *testing.B) {
	client := controlflow.Require_ServerToClient(controlflow.NewRequire())
	defer client.Release()
	b.ReportAllocs()

	for b.Loop() {
		err := client.Write(context.Background(), func(params controlflow.Require_write_Params) error {
			params.SetTest(true)

			if err := params.SetReason("ordered observations are required"); err != nil {
				return err
			}
			return params.SetData([]byte(`{"sequence":9007199254740993}`))
		})

		if err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		_, err = future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}
