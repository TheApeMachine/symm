package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestColumns(t *testing.T) {
	ctx := context.Background()

	Convey("Given named columns", t, func() {
		client := data.Columns_ServerToClient(data.NewColumns())
		defer client.Release()

		zip := func(names string, columns ...string) (string, bool) {
			So(client.Write(ctx, func(params data.Columns_write_Params) error {
				if err := params.SetNames(names); err != nil {
					return err
				}

				if err := params.SetIndex("id"); err != nil {
					return err
				}

				list, err := params.NewColumns(int32(len(columns)))

				if err != nil {
					return err
				}

				for index, column := range columns {
					if err := list.Set(index, []byte(column)); err != nil {
						return err
					}
				}

				return nil
			}), ShouldBeNil)

			if err := client.WaitStreaming(); err != nil {
				return err.Error(), false
			}

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			if results.Which() != data.Rows_Which_out {
				return "", false
			}

			out, err := results.Out()
			So(err, ShouldBeNil)
			return string(out), true
		}

		Convey("Rows hold element i of every column, with their position", func() {
			rows, ok := zip("x,label", `[1,2]`, `["a","b"]`)
			So(ok, ShouldBeTrue)
			So(rows, ShouldEqual, `[{"id":0,"label":"a","x":1},{"id":1,"label":"b","x":2}]`)
		})

		Convey("A column that has not arrived leaves it idle", func() {
			_, ok := zip("x,label", `[1,2]`, ``)
			So(ok, ShouldBeFalse)
		})

		Convey("Columns of different lengths are rejected", func() {
			message, ok := zip("x,label", `[1,2]`, `["a"]`)
			So(ok, ShouldBeFalse)
			So(message, ShouldContainSubstring, "elements")
		})
	})
}
