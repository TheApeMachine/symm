package paper_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/paper"
)

func sweep(levels, amount string, spend bool) (quantity, cost, unfilled string, err error) {
	ctx := context.Background()
	client := paper.Sweep_ServerToClient(paper.NewSweep(context.Background()))
	defer client.Release()

	future, release := client.Execute(ctx, func(params paper.Sweep_execute_Params) error {
		params.SetSpend(spend)
		var values [][2]string
		if err := json.Unmarshal([]byte(levels), &values); err != nil {
			return err
		}
		list, err := params.NewLevels(int32(len(values)))
		if err != nil {
			return err
		}
		for index, value := range values {
			if err := list.At(index).SetPrice(value[0]); err != nil {
				return err
			}
			if err := list.At(index).SetQuantity(value[1]); err != nil {
				return err
			}
		}

		for _, err := range []error{
			params.SetAmount(amount),
			params.SetIncrement("0.0001"),
		} {
			if err != nil {
				return err
			}
		}
		return nil
	})
	defer release()
	results, err := future.Struct()

	if err != nil {
		return "", "", "", err
	}
	read := func(value string, err error) string {
		So(err, ShouldBeNil)
		return value
	}
	return read(results.Quantity()), read(results.Cost()), read(results.Unfilled()), nil
}

func TestSweepExecute(t *testing.T) {
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
