package cognition_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestLearning(t *testing.T) {
	ctx := context.Background()

	Convey("Given a writer reinforcing what it observes", t, func() {
		writer := cognition.NewReinforce(ctx)
		reader := cognition.NewAttractor(ctx)

		// One learned memory, wired to both, the way the graph wires it.
		memory := cognition.Memory_ServerToClient(cognition.NewMemory())
		defer memory.Release()

		reinforce := func(basin, class string) []byte {
			client := cognition.Reinforce_ServerToClient(writer)

			err := client.Write(ctx, func(params cognition.Reinforce_write_Params) error {
				if err := params.SetContextBytes([]byte(basin)); err != nil {
					return err
				}

				if err := params.SetClassBytes([]byte(class)); err != nil {
					return err
				}

				return params.SetMemory(memory.AddRef())
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			key, err := results.Out()
			So(err, ShouldBeNil)

			return bytes.Clone(key)
		}

		attract := func(basin string) (string, float64, int64) {
			client := cognition.Attractor_ServerToClient(reader)

			err := client.Write(ctx, func(params cognition.Attractor_write_Params) error {
				if err := params.SetContextBytes([]byte(basin)); err != nil {
					return err
				}

				return params.SetMemory(memory.AddRef())
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			class, err := results.Class()
			So(err, ShouldBeNil)

			return string(class), results.Prob(), results.Count()
		}

		Convey("A reader settles on the class observed most in a basin", func() {
			reinforce("rising", "enter")
			reinforce("rising", "enter")
			reinforce("rising", "enter")
			reinforce("rising", "wait")

			class, probability, observed := attract("rising")
			So(class, ShouldEqual, "enter")
			So(observed, ShouldEqual, 2)
			So(probability, ShouldAlmostEqual, 0.75, 1e-9)
		})

		Convey("A basin never observed attracts to nothing", func() {
			reinforce("rising", "enter")

			class, probability, observed := attract("falling")
			So(class, ShouldEqual, "")
			So(probability, ShouldEqual, 0)
			So(observed, ShouldEqual, 0)
		})

		Convey("Reinforcement accumulates rather than replacing", func() {
			reinforce("quiet", "wait")
			reinforce("quiet", "wait")

			// What the memory carries is read back through the memory, not
			// through a structure the writer kept to itself.
			future, release := memory.Basin(ctx, func(params cognition.Memory_basin_Params) error {
				return params.SetPrefix(cognition.BasinPrefixOf([]byte("quiet")))
			})
			defer release()

			basin, err := future.Struct()
			So(err, ShouldBeNil)

			weights, err := basin.Weights()
			So(err, ShouldBeNil)
			So(weights.Len(), ShouldEqual, 1)
			So(weights.At(0), ShouldEqual, 2)
		})

		Convey("A basin key emitted by the writer is accepted as a context", func() {
			key := reinforce("rising", "enter")
			So(string(key), ShouldEqual, "b/rising/enter")

			class, _, _ := attract(string(key))
			So(class, ShouldEqual, "enter")
		})

		Convey("Every class in a basin is counted", func() {
			reinforce("open", "enter")
			reinforce("open", "wait")
			reinforce("open", "exit")

			_, probability, observed := attract("open")
			So(observed, ShouldEqual, 3)
			So(probability, ShouldAlmostEqual, 1.0/3.0, 1e-9)
		})
	})
}
