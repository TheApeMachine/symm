package data

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSeriesWrite(t *testing.T) {
	Convey("Given a series retaining samples per key", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Series_ServerToClient(NewSeries())
		defer client.Release()

		/*
			record retains one sample under a key, which is what a metric does
			as each symbol's observations arrive.
		*/
		record := func(key string, sec, value float64) {
			err := client.Write(ctx, func(params Series_write_Params) error {
				if err := params.SetKey(key); err != nil {
					return err
				}

				params.SetSec(sec)
				params.SetValue(value)
				params.SetQuery(false)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			_, err = future.Struct()
			So(err, ShouldBeNil)
		}

		/*
			query reads a key back without retaining anything.
		*/
		query := func(key string) (value float64, found bool) {
			err := client.Write(ctx, func(params Series_write_Params) error {
				if err := params.SetKey(key); err != nil {
					return err
				}

				params.SetQuery(true)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results.Value(), results.Found()
		}

		Convey("When several keys are retained and then read back", func() {
			record("BTC", 1, 100)
			record("ETH", 1, 20)
			record("BTC", 2, 101)

			Convey("Then each key reports its own newest sample", func() {
				value, found := query("BTC")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 101)

				value, found = query("ETH")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 20)
			})
		})

		Convey("When a key nothing was written under is read", func() {
			record("BTC", 1, 100)

			Convey("Then it is reported as absent rather than as a zero sample", func() {
				value, found := query("SOL")
				So(found, ShouldBeFalse)
				So(value, ShouldEqual, 0)
			})
		})

		Convey("When a query is issued", func() {
			record("BTC", 1, 100)
			query("BTC")

			Convey("Then the query itself retains nothing", func() {
				value, found := query("BTC")
				So(found, ShouldBeTrue)
				So(value, ShouldEqual, 100)
			})
		})
	})
}
