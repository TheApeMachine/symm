package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestPluck(t *testing.T) {
	ctx := context.Background()

	Convey("Given documents handed over together", t, func() {
		client := data.Pluck_ServerToClient(data.NewPluck())
		defer client.Release()

		pluck := func(documents ...string) string {
			So(client.Write(ctx, func(params data.Pluck_write_Params) error {
				if err := params.SetPath("symbol"); err != nil {
					return err
				}

				list, err := params.NewData(int32(len(documents)))

				if err != nil {
					return err
				}

				for index, document := range documents {
					if err := list.Set(index, []byte(document)); err != nil {
						return err
					}
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()
			results, err := future.Struct()
			So(err, ShouldBeNil)

			if results.Which() == data.Plucked_Which_idle {
				return ""
			}

			out, err := results.Out()
			So(err, ShouldBeNil)
			return string(out)
		}

		Convey("The field of each is collected in order, skipping documents without it", func() {
			So(pluck(`{"symbol":"BTC/USD"}`, `{"quote":"USD"}`, `{"symbol":"ETH/USD"}`), ShouldEqual, `["BTC/USD","ETH/USD"]`)

			Convey("And an evaluation plucking nothing is idle", func() {
				So(pluck(`{"quote":"USD"}`), ShouldEqual, "")
			})
		})
	})
}
