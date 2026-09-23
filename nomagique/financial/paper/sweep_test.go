package paper_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
)

func sweep(levels, amount string, spend bool) (quantity, cost, unfilled string, err error) {
	ctx := context.Background()
	client := paper.Sweep_ServerToClient(paper.NewSweep(context.Background()))
	defer client.Release()

	if err := client.Write(ctx, func(params paper.Sweep_write_Params) error {
		params.SetSpend(spend)

		for _, err := range []error{
			params.SetLevels([]byte(levels)),
			params.SetAmount([]byte(amount)),
			params.SetIncrement([]byte("0.0001")),
		} {
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", "", "", err
	}

	if err := client.WaitStreaming(); err != nil {
		return "", "", "", err
	}
	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		return "", "", "", err
	}
	read := func(value []byte, err error) string {
		So(err, ShouldBeNil)
		return string(bytes.Clone(value))
	}
	return read(results.Quantity()), read(results.Cost()), read(results.Unfilled()), nil
}

func TestSweepWrite(t *testing.T) {
	Convey("Given the asks 1 @ 100 and 2 @ 101", t, func() {
		Convey("When a quote budget is spent into them", func() {
			quantity, cost, unfilled, err := sweep(`[["100","1"],["101","2"]]`, "199.20318", true)
			So(err, ShouldBeNil)

			Convey("Then whole levels are bought and the last in whole increments", func() {
				So(quantity, ShouldEqual, "1.9822")
				So(cost, ShouldEqual, "199.2022")
				So(unfilled, ShouldEqual, "0.00098")
			})
		})

		Convey("When the budget exceeds the visible book", func() {
			quantity, _, unfilled, err := sweep(`[["100","1"],["101","2"]]`, "1000", true)
			So(err, ShouldBeNil)

			Convey("Then what the book cannot absorb stays unfilled", func() {
				So(quantity, ShouldEqual, "3")
				So(unfilled, ShouldEqual, "698")
			})
		})
	})

	Convey("Given the bids 1 @ 99 and 3 @ 98", t, func() {
		quantity, cost, unfilled, err := sweep(`[["99","1"],["98","3"]]`, "1.9822", false)
		So(err, ShouldBeNil)

		Convey("Then a delivered quantity walks them down", func() {
			So(quantity, ShouldEqual, "1.9822")
			So(cost, ShouldEqual, "195.2556")
			So(unfilled, ShouldEqual, "0")
		})
	})
}
