package cognition_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestTokenSequence(t *testing.T) {
	Convey("Given a TokenSequence capability", t, func() {
		ctx := context.Background()
		server := cognition.NewTokenSequence()
		So(server, ShouldNotBeNil)

		client := cognition.TokenSequence_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		step := func(scope, token string, reset bool) ([]string, string, int64) {
			err := client.Write(ctx, func(params cognition.TokenSequence_write_Params) error {
				if err := params.SetScope(scope); err != nil {
					return err
				}
				if err := params.SetToken(token); err != nil {
					return err
				}
				params.SetReset(reset)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			path, err := results.Path()
			So(err, ShouldBeNil)

			depth := results.Depth()

			seqList, err := results.Sequence()
			So(err, ShouldBeNil)

			tokens := make([]string, seqList.Len())
			for index := range seqList.Len() {
				tok, err := seqList.At(index)
				So(err, ShouldBeNil)
				tokens[index] = tok
			}

			return tokens, path, depth
		}

		Convey("Sequential tokens extend the retained precursor path", func() {
			tokens, path, depth := step("BTC/USD", "R1", false)
			So(tokens, ShouldResemble, []string{"R1"})
			So(path, ShouldEqual, "R1")
			So(depth, ShouldEqual, 1)

			tokens, path, depth = step("BTC/USD", "R4", false)
			So(tokens, ShouldResemble, []string{"R1", "R4"})
			So(path, ShouldEqual, "R1/R4")
			So(depth, ShouldEqual, 2)

			tokens, path, depth = step("BTC/USD", "[R7,R9]", false)
			So(tokens, ShouldResemble, []string{"R1", "R4", "[R7,R9]"})
			So(path, ShouldEqual, "R1/R4/[R7,R9]")
			So(depth, ShouldEqual, 3)
		})

		Convey("Two different histories ending at the same current token remain separate", func() {
			// History 1: R1 -> R4 -> [R7,R9]
			step("sym1", "R1", false)
			step("sym1", "R4", false)
			_, path1, _ := step("sym1", "[R7,R9]", false)

			// History 2: R3 -> R2 -> [R7,R9]
			step("sym2", "R3", false)
			step("sym2", "R2", false)
			_, path2, _ := step("sym2", "[R7,R9]", false)

			So(path1, ShouldEqual, "R1/R4/[R7,R9]")
			So(path2, ShouldEqual, "R3/R2/[R7,R9]")
			So(path1, ShouldNotEqual, path2)
		})

		Convey("Reset clears prior token history for that scope", func() {
			step("ETH/USD", "R1", false)
			step("ETH/USD", "R2", false)

			tokens, path, depth := step("ETH/USD", "R5", true)
			So(tokens, ShouldResemble, []string{"R5"})
			So(path, ShouldEqual, "R5")
			So(depth, ShouldEqual, 1)
		})
	})
}
