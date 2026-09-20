package cognition_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestCognitionPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given cognition primitives", t, func() {
		Convey("Associate pairs precursor and current observations", func() {
			server := cognition.NewAssociate()
			client := cognition.Associate_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params cognition.Associate_write_Params) error {
				return params.SetCurrent([]byte("obs1"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			prec, _ := results.Precursor()
			curr, _ := results.Current()
			So(string(prec), ShouldEqual, "obs1")
			So(curr, ShouldBeNil)

			Convey("When a second observation arrives, precursor is obs1 and current is obs2", func() {
				err = client.Write(ctx, func(params cognition.Associate_write_Params) error {
					return params.SetCurrent([]byte("obs2"))
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				secondPrec, _ := secondResults.Precursor()
				secondCurr, _ := secondResults.Current()
				So(string(secondPrec), ShouldEqual, "obs1")
				So(string(secondCurr), ShouldEqual, "obs2")
			})
		})

		Convey("BasinKey formats basin path from class and contextBytes", func() {
			server := cognition.NewBasinKey()
			client := cognition.BasinKey_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params cognition.BasinKey_write_Params) error {
				_ = params.SetClass([]byte("bull"))
				return params.SetContextBytes([]byte("trend"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			out, _ := results.Out()
			So(string(out), ShouldEqual, "b/trend/bull")
		})

		Convey("ParseBasinKey decomposes basin path into class and context", func() {
			server := cognition.NewParseBasinKey()
			client := cognition.ParseBasinKey_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params cognition.ParseBasinKey_write_Params) error {
				return params.SetKey([]byte("b/trend/bull"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			class, _ := results.Class()
			contextBytes, _ := results.ContextBytes()
			So(results.Ok(), ShouldBeTrue)
			So(string(class), ShouldEqual, "bull")
			So(string(contextBytes), ShouldEqual, "trend")
		})

		Convey("Pack and Weight serialize and deserialize [3]uint64", func() {
			packServer := cognition.NewPack()
			packClient := cognition.Pack_ServerToClient(packServer)

			err := packClient.Write(ctx, func(params cognition.Pack_write_Params) error {
				params.SetCount(10)
				params.SetMass(500)
				params.SetWriteStep(42)
				return nil
			})
			So(err, ShouldBeNil)
			So(packClient.WaitStreaming(), ShouldBeNil)

			future, release := packClient.Done(ctx, nil)
			defer release()

			packResults, err := future.Struct()
			So(err, ShouldBeNil)
			data, _ := packResults.Out()

			weightServer := cognition.NewWeight()
			weightClient := cognition.Weight_ServerToClient(weightServer)

			err = weightClient.Write(ctx, func(params cognition.Weight_write_Params) error {
				return params.SetRecord(data)
			})
			So(err, ShouldBeNil)
			So(weightClient.WaitStreaming(), ShouldBeNil)

			weightFuture, weightRelease := weightClient.Done(ctx, nil)
			defer weightRelease()

			weightResults, err := weightFuture.Struct()
			So(err, ShouldBeNil)
			So(weightResults.Count(), ShouldEqual, 10)
			So(weightResults.Mass(), ShouldEqual, 500)
			So(weightResults.WriteStep(), ShouldEqual, 42)
		})
	})
}
