package strategy

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/theapemachine/symm/broker"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func virtualFixture() (virtualWallet, *spotbook.Book) {
	wallet := virtualWallet{}
	err := wallet.initialize(decimal.NewFromInt64(1000), kraken.InstrumentPair{Symbol: "TEST/USD",
		QtyIncrement: decimal.NewFromInt64(1), QtyMin: decimal.NewFromInt64(1), CostMin: decimal.NewFromInt64(1)}, decimal.NewFromInt64(1))
	if err != nil {
		panic(err)
	}
	book := spotbook.New()
	book.NoBookCrossing = false

	for _, level := range []struct {
		direction       spotbook.BookDirection
		id              string
		price, quantity int64
	}{
		{spotbook.Bid, "bid", 100, 10}, {spotbook.Ask, "ask", 101, 3}, {spotbook.Ask, "deep", 102, 10},
	} {
		book.Update(&spotbook.UpdateOptions{Direction: level.direction, ID: level.id,
			Price: decimal.NewFromInt64(level.price), Quantity: decimal.NewFromInt64(level.quantity), Silent: true})
	}
	return wallet, book
}

func TestVirtualWalletSweep(t *testing.T) {
	Convey("Given cash for exactly one lot including its fee", t, func() {
		wallet, book := virtualFixture()
		wallet.cash.SetFrac64(10201, 100)
		requested := big.NewRat(3, 1)
		quantity, gross := wallet.pricing.Sweep(book, requested, &wallet.cash, true, nil, nil)
		So(quantity.RatString(), ShouldEqual, "1")
		So(gross.RatString(), ShouldEqual, "101")

		Convey("More expensive depth cannot fill another lot or add scan work", func() {
			// A populated tail exposes accidental walks after cash is exhausted.
			const inaccessibleLevels = 512
			for depth := range inaccessibleLevels {
				book.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask,
					ID: fmt.Sprint(depth), Price: decimal.NewFromInt64(int64(103 + depth)),
					Quantity: decimal.NewFromInt64(1), Silent: true})
			}
			quantity, gross = wallet.pricing.Sweep(book, requested, &wallet.cash, true, nil, nil)
			So(quantity.RatString(), ShouldEqual, "1")
			So(gross.RatString(), ShouldEqual, "101")
			deepAllocations := testing.AllocsPerRun(1, func() {
				wallet.pricing.Sweep(book, requested, &wallet.cash, true, nil, nil)
			})
			// Converting each price allocates a Rat. Fewer allocations than
			// inaccessible levels rules out that walk without exact pool counts,
			// which vary under the race detector.
			So(deepAllocations, ShouldBeLessThan, inaccessibleLevels)
		})
	})
}

func TestVirtualWalletFill(t *testing.T) {
	Convey("Given an independent wallet and two executable ask levels", t, func() {
		wallet, book := virtualFixture()
		other, _ := virtualFixture()
		enter := LearningAction{Kind: types.ActionEnter}
		quantity, gross, fee := wallet.fill(book, enter, big.NewRat(5, 1), nil)
		So(quantity.FloatString(2), ShouldEqual, "5.00")
		So(gross.FloatString(2), ShouldEqual, "507.00")
		So(fee.FloatString(2), ShouldEqual, "5.07")
		So(wallet.cash.FloatString(2), ShouldEqual, "487.93")
		So(other.cash.FloatString(2), ShouldEqual, "1000.00")
		So(other.quantity.Sign(), ShouldEqual, 0)

		Convey("Liquidation includes spread, depth and both sides' fees", func() {
			mark, complete := wallet.mark(book)
			So(complete, ShouldBeTrue)
			So(mark.FloatString(2), ShouldEqual, "982.93")
			book.Update(&spotbook.UpdateOptions{Direction: spotbook.Bid, ID: "lift", Price: decimal.NewFromInt64(110), Quantity: decimal.NewFromInt64(5), Silent: true})
			quantity, gross, fee = wallet.fill(book, LearningAction{Kind: types.ActionExit, Reduce: true}, &wallet.quantity, nil)
			So(quantity.FloatString(2), ShouldEqual, "5.00")
			So(gross.FloatString(2), ShouldEqual, "550.00")
			So(fee.FloatString(2), ShouldEqual, "5.50")
			So(wallet.quantity.Sign(), ShouldEqual, 0)
			So(wallet.cash.FloatString(2), ShouldEqual, "1032.43")
			So(wallet.fees.FloatString(2), ShouldEqual, "10.57")
		})

		Convey("A later withdrawal produces a partial IOC without inventing depth", func() {
			book.Update(&spotbook.UpdateOptions{Direction: spotbook.Bid, ID: "bid", Price: decimal.NewFromInt64(100), Quantity: decimal.NewFromInt64(2), Silent: true})
			_, complete := wallet.mark(book)
			So(complete, ShouldBeFalse)
			quantity, _, _ = wallet.fill(book, LearningAction{Kind: types.ActionExit, Reduce: true}, &wallet.quantity, nil)
			So(quantity.FloatString(2), ShouldEqual, "2.00")
			So(wallet.quantity.FloatString(2), ShouldEqual, "3.00")
		})

		Convey("A wait changes neither the account nor its fees", func() {
			before := wallet.cash.String()
			quantity, _, fee = wallet.fill(book, LearningAction{Kind: types.ActionHold}, new(big.Rat), nil)
			So(quantity.Sign(), ShouldEqual, 0)
			So(fee.Sign(), ShouldEqual, 0)
			So(wallet.cash.String(), ShouldEqual, before)
		})
	})
}

func TestVirtualWalletActions(t *testing.T) {
	Convey("Given whole-unit cash and a token quoted below that cash precision", t, func() {
		parse := func(value string) *decimal.Decimal {
			parsed, err := decimal.NewFromString(value)
			So(err, ShouldBeNil)
			return parsed
		}
		wallet := virtualWallet{}
		err := wallet.initialize(parse("1"), kraken.InstrumentPair{Symbol: "SMALL/USD",
			QtyIncrement: parse("1"), QtyMin: parse("1"), CostMin: parse("0.0000001"),
			PricePrecision: 10}, parse("0.1"))
		So(err, ShouldBeNil)
		book := spotbook.New()
		book.Update(&spotbook.UpdateOptions{Direction: spotbook.Bid, ID: "bid", Price: parse("0.0000000010"), Quantity: parse("1000000"), Silent: true})
		book.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask, ID: "ask", Price: parse("0.0000000011"), Quantity: parse("1000000"), Silent: true})

		Convey("Sizing and fills preserve the exact price, gross and fee", func() {
			So(wallet.maximum(book, true).FloatString(0), ShouldEqual, "1000000")
			So(wallet.actions(book, nil), ShouldNotBeEmpty)
			quantity, gross, fee := wallet.fill(book, LearningAction{Kind: types.ActionEnter}, big.NewRat(1000, 1), nil)
			So(quantity.FloatString(0), ShouldEqual, "1000")
			So(gross.FloatString(10), ShouldEqual, "0.0000011000")
			So(fee.FloatString(10), ShouldEqual, "0.0000000011")
			So(wallet.cash.FloatString(10), ShouldEqual, "0.9999988989")
			mark, complete := wallet.mark(book)
			So(complete, ShouldBeTrue)
			So(mark.FloatString(10), ShouldEqual, "0.9999998979")
		})
	})

	Convey("Given available cash, venue lot units and minimum order value", t, func() {
		wallet, book := virtualFixture()
		actions := wallet.actions(book, nil)
		So(actions, ShouldResemble, []LearningAction{{Kind: types.ActionHold},
			{Kind: types.ActionEnter}, {Kind: types.ActionEnter, Power: 1},
			{Kind: types.ActionEnter, Power: 2}, {Kind: types.ActionEnter, Power: 3}})

		Convey("Each selected refinement is feasible and cannot borrow cash", func() {
			for _, action := range actions {
				independent, _ := virtualFixture()
				requested := independent.request(book, action, 1, nil)
				quantity, _, _ := independent.fill(book, action, requested, nil)
				So(quantity.Cmp(requested), ShouldBeLessThanOrEqualTo, 0)
				So(independent.cash.Sign(), ShouldBeGreaterThanOrEqualTo, 0)
			}
		})

		Convey("An unaffordable minimum leaves only wait", func() {
			wallet.pricing.CostMinimum.SetInt64(2000)
			So(wallet.actions(book, nil), ShouldResemble, []LearningAction{{Kind: types.ActionHold}})
		})
	})
}

func BenchmarkVirtualWalletActions(b *testing.B) {
	for _, depth := range []int{2, 512} {
		b.Run(fmt.Sprint(depth), func(b *testing.B) {
			wallet, book := virtualFixture()

			for offset := 2; offset < depth; offset++ {
				book.Update(&spotbook.UpdateOptions{Direction: spotbook.Ask,
					ID: fmt.Sprint(offset), Price: decimal.NewFromInt64(int64(101 + offset)),
					Quantity: decimal.NewFromInt64(10), Silent: true})
			}
			actions := wallet.actions(book, nil)
			b.ReportAllocs()

			for b.Loop() {
				actions = wallet.actions(book, actions)
			}
		})
	}
}

func TestFillCannotTakeLiquidityItNeverRaced(t *testing.T) {
	Convey("Given a decision sized against the depth displayed at the time", t, func() {
		wallet, book := virtualFixture()
		action := LearningAction{Kind: types.ActionEnter}
		ladder := &broker.DepthLadder{}
		requested := wallet.request(book, action, 1, ladder)

		So(requested.Sign(), ShouldBeGreaterThan, 0)
		So(ladder.Count, ShouldBeGreaterThan, 0)

		Convey("Depth that appeared after the decision cannot be filled against", func() {
			// The observed ladder is emptied of every price the fill will walk,
			// so nothing on the current book was standing when the decision was
			// made. An order racing a book like that gets nothing.
			vanished := &broker.DepthLadder{}
			for level := book.Asks.Low; level != nil; level = level.Higher {
				vanished.Record(level.Price.Rat(), new(big.Rat))
			}

			quantity, gross, fee := wallet.fill(book, action, requested, vanished)
			So(quantity.Sign(), ShouldEqual, 0)
			So(gross.Sign(), ShouldEqual, 0)
			So(fee.Sign(), ShouldEqual, 0)
			So(wallet.quantity.Sign(), ShouldEqual, 0)
		})

		Convey("Only the surviving share of each level is filled", func() {
			// Half of every level was cancelled between decision and execution.
			thinned := &broker.DepthLadder{}
			for level := book.Asks.Low; level != nil; level = level.Higher {
				half := new(big.Rat).Quo(level.Quantity.Rat(), big.NewRat(2, 1))
				thinned.Record(level.Price.Rat(), half)
			}

			partial, _, _ := wallet.fill(book, action, requested, thinned)
			So(partial.Sign(), ShouldBeGreaterThan, 0)
			So(partial.Cmp(requested), ShouldBeLessThan, 0)
		})

		Convey("An unchanged book fills exactly what an unrestricted sweep would", func() {
			restricted, _ := virtualFixture()
			unrestricted, _ := virtualFixture()

			capped, _, _ := restricted.fill(book, action, requested, ladder)
			open, _, _ := unrestricted.fill(book, action, requested, nil)
			So(capped.Cmp(open), ShouldEqual, 0)
		})
	})
}
