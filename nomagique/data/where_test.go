package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestWhere(t *testing.T) {
	ctx := context.Background()

	Convey("Given documents handed over together", t, func() {
		client := data.Where_ServerToClient(data.NewWhere())
		defer client.Release()

		where := func(value string, documents ...string) []string {
			So(client.Write(ctx, func(params data.Where_write_Params) error {
				if err := params.SetPath("market.channel"); err != nil {
					return err
				}

				if err := params.SetValue([]byte(value)); err != nil {
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

			kept := []string{}

			if results.Which() == data.Kept_Which_idle {
				return kept
			}

			out, err := results.Out()
			So(err, ShouldBeNil)

			for index := range out.Len() {
				document, err := out.At(index)
				So(err, ShouldBeNil)
				kept = append(kept, string(document))
			}

			return kept
		}

		Convey("Only the ones whose value at the path matches are kept, in order", func() {
			kept := where(`"instrument"`,
				`{"market":{"channel":"ticker"}}`,
				`{"market":{"channel":"instrument","n":1}}`,
				`{"cursor":{}}`,
				`{"market":{"channel":"instrument","n":2}}`,
			)
			So(kept, ShouldResemble, []string{
				`{"market":{"channel":"instrument","n":1}}`,
				`{"market":{"channel":"instrument","n":2}}`,
			})

			Convey("And an evaluation keeping nothing is idle", func() {
				So(where(`"instrument"`, `{"market":{"channel":"ticker"}}`), ShouldBeEmpty)
			})
		})

		Convey("A document that is not JSON is rejected", func() {
			So(client.Write(ctx, func(params data.Where_write_Params) error {
				if err := params.SetPath("a"); err != nil {
					return err
				}

				if err := params.SetValue([]byte(`1`)); err != nil {
					return err
				}

				list, err := params.NewData(1)

				if err != nil {
					return err
				}

				return list.Set(0, []byte("not json"))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
