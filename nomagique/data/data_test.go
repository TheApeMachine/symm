package data_test

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDataPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given data primitives", t, func() {
		Convey("Extract reads a scalar at a dotted path", func() {
			client := data.Extract_ServerToClient(data.NewExtract(ctx))
			So(client.IsValid(), ShouldBeTrue)

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 42.5, "qty": "1.25"},
				"levels": []any{
					map[string]any{"price": 99.0},
					map[string]any{"price": 98.0},
				},
			})
			So(err, ShouldBeNil)

			read := func(path string) (float64, bool) {
				err := client.Write(ctx, func(params data.Extract_write_Params) error {
					if err := params.SetPath(path); err != nil {
						return err
					}

					return params.SetData(payload)
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				future, release := client.Done(ctx, nil)
				defer release()

				results, err := future.Struct()
				So(err, ShouldBeNil)

				if !results.Found() {
					So(results.Which(), ShouldEqual, data.Extracted_Which_missing)
					return 0, false
				}
				return results.Out(), true
			}

			Convey("It resolves a nested path", func() {
				value, found := read("trade.price")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 42.5)
			})

			Convey("It parses a venue decimal carried as a string", func() {
				value, found := read("trade.qty")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 1.25)
			})

			Convey("It indexes an array segment", func() {
				value, found := read("levels.1.price")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 98.0)
			})

			Convey("It reports an absent path as not found rather than zero", func() {
				_, found := read("trade.missing")
				So(found, ShouldBeFalse)
			})

			Convey("It reports an out of range index as not found", func() {
				_, found := read("levels.9.price")
				So(found, ShouldBeFalse)
			})
		})

		Convey("Quality computes SNR and maturity from scalar fields", func() {
			server := data.NewQuality()
			client := data.Quality_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params data.Quality_write_Params) error {
				params.SetSupport(10.0)
				params.SetDivergence(2.0)
				params.SetNoiseVariance(1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			So(results.Estimated(), ShouldBeTrue)
			So(results.SnrDefined(), ShouldBeTrue)
			So(results.Snr(), ShouldEqual, 4.0)
			So(results.Maturity(), ShouldAlmostEqual, 0.9, 1e-6)
		})
	})
}
