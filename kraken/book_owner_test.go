package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func mustDecimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return parsed
}

func bid(orderID, price, qty string) Level3Order {
	return Level3Order{
		OrderID:    orderID,
		LimitPrice: mustDecimal(price),
		OrderQty:   mustDecimal(qty),
	}
}

func ask(orderID, price, qty string) Level3Order {
	return bid(orderID, price, qty)
}

func snapshot(symbol string, bids, asks []Level3Order) Level3Data {
	return Level3Data{
		Symbol: symbol,
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	}
}

func update(symbol string, bids, asks []Level3Order) Level3Data {
	return Level3Data{
		Symbol: symbol,
		Type:   "update",
		Bids:   bids,
		Asks:   asks,
	}
}

func TestBookOwnerSnapshot(t *testing.T) {
	Convey("Given a canonical book owner", t, func() {
		owner := NewBookOwner()

		Convey("an initial snapshot replaces and initializes the symbol book", func() {
			owner.Apply(snapshot("X/USD",
				[]Level3Order{bid("b1", "100", "2"), bid("b2", "99", "5")},
				[]Level3Order{ask("a1", "101", "3"), ask("a2", "102", "7")},
			))

			found := owner.Fold("X/USD", func(view BookView) {
				So(view.Valid, ShouldBeTrue)
				So(len(view.Bids), ShouldEqual, 2)
				So(len(view.Asks), ShouldEqual, 2)

				Convey("bids are ordered descending and asks ascending", func() {
					So(view.Bids[0].OrderID, ShouldEqual, "b1")
					So(view.Bids[1].OrderID, ShouldEqual, "b2")
					So(view.Asks[0].OrderID, ShouldEqual, "a1")
					So(view.Asks[1].OrderID, ShouldEqual, "a2")
				})
			})

			So(found, ShouldBeTrue)
		})
	})
}

func TestBookOwnerIncrementalAddModifyDelete(t *testing.T) {
	Convey("Given a snapshot then incremental updates", t, func() {
		owner := NewBookOwner()
		owner.Apply(snapshot("X/USD",
			[]Level3Order{bid("b1", "100", "2")},
			[]Level3Order{ask("a1", "101", "3")},
		))

		Convey("add inserts a new order at the correct price position", func() {
			owner.Apply(update("X/USD",
				[]Level3Order{{OrderID: "b0", Event: "add", LimitPrice: mustDecimal("101.5"), OrderQty: mustDecimal("1")}},
				nil,
			))

			owner.Fold("X/USD", func(view BookView) {
				So(len(view.Bids), ShouldEqual, 2)
				So(view.Bids[0].OrderID, ShouldEqual, "b0")
			})
		})

		Convey("modify replaces quantity/price in place preserving order identity", func() {
			owner.Apply(update("X/USD",
				[]Level3Order{{OrderID: "b1", Event: "modify", LimitPrice: mustDecimal("100"), OrderQty: mustDecimal("9")}},
				nil,
			))

			owner.Fold("X/USD", func(view BookView) {
				So(len(view.Bids), ShouldEqual, 1)
				So(view.Bids[0].OrderID, ShouldEqual, "b1")
				So(view.Bids[0].OrderQty.Cmp(mustDecimal("9")), ShouldEqual, 0)
			})
		})

		Convey("delete removes the order by identity", func() {
			owner.Apply(update("X/USD",
				[]Level3Order{{OrderID: "b1", Event: "delete"}},
				nil,
			))

			owner.Fold("X/USD", func(view BookView) {
				So(len(view.Bids), ShouldEqual, 0)
				So(view.Valid, ShouldBeFalse)
			})
		})
	})
}

func TestBookOwnerValidity(t *testing.T) {
	Convey("Given a book owner", t, func() {
		owner := NewBookOwner()

		Convey("a one-sided book is invalid, never repaired", func() {
			owner.Apply(snapshot("X/USD",
				[]Level3Order{bid("b1", "100", "2")},
				nil,
			))

			owner.Fold("X/USD", func(view BookView) {
				So(view.Valid, ShouldBeFalse)
				So(len(view.Bids), ShouldEqual, 1)
				So(len(view.Asks), ShouldEqual, 0)
			})
		})

		Convey("an empty book is invalid", func() {
			owner.Apply(snapshot("X/USD", nil, nil))

			owner.Fold("X/USD", func(view BookView) {
				So(view.Valid, ShouldBeFalse)
			})
		})

		Convey("a crossed book is invalid and not repaired", func() {
			owner.Apply(snapshot("X/USD",
				[]Level3Order{bid("b1", "101", "2")},
				[]Level3Order{ask("a1", "100", "3")},
			))

			owner.Fold("X/USD", func(view BookView) {
				So(view.Valid, ShouldBeFalse)
				So(len(view.Bids), ShouldEqual, 1)
				So(len(view.Asks), ShouldEqual, 1)
			})
		})

		Convey("a crossed book that deletes the crossing ask becomes valid", func() {
			owner.Apply(snapshot("X/USD",
				[]Level3Order{bid("b1", "101", "2")},
				[]Level3Order{ask("a1", "100", "3"), ask("a2", "102", "4")},
			))

			owner.Apply(update("X/USD", nil,
				[]Level3Order{{OrderID: "a1", Event: "delete"}},
			))

			owner.Fold("X/USD", func(view BookView) {
				So(view.Valid, ShouldBeTrue)
			})
		})
	})
}

func TestBookOwnerFixedPointPrecision(t *testing.T) {
	Convey("Given fixed-point order values", t, func() {
		owner := NewBookOwner()
		owner.Apply(snapshot("X/USD",
			[]Level3Order{bid("b1", "0.5634", "2400.5")},
			[]Level3Order{ask("a1", "0.5640", "3500.7766862600")},
		))

		Convey("resident state preserves the fixed-point decimal values", func() {
			owner.Fold("X/USD", func(view BookView) {
				So(view.Bids[0].LimitPrice.Cmp(mustDecimal("0.5634")), ShouldEqual, 0)
				So(view.Bids[0].OrderQty.Cmp(mustDecimal("2400.5")), ShouldEqual, 0)
				So(view.Asks[0].LimitPrice.Cmp(mustDecimal("0.5640")), ShouldEqual, 0)
				So(view.Asks[0].OrderQty.Cmp(mustDecimal("3500.7766862600")), ShouldEqual, 0)
			})
		})
	})
}

func TestBookOwnerFoldReturnsFalseForUnknownSymbol(t *testing.T) {
	Convey("Given a book owner with no symbol", t, func() {
		owner := NewBookOwner()

		Convey("Fold reports no book without invoking the callback", func() {
			called := false
			found := owner.Fold("NOPE/USD", func(BookView) { called = true })

			So(found, ShouldBeFalse)
			So(called, ShouldBeFalse)
		})
	})
}

func TestBookOwnerCausalOrder(t *testing.T) {
	Convey("Given update frames applying in causal order", t, func() {
		owner := NewBookOwner()
		owner.Apply(snapshot("X/USD",
			[]Level3Order{bid("b1", "100", "2")},
			[]Level3Order{ask("a1", "101", "3")},
		))

		owner.Apply(update("X/USD",
			[]Level3Order{{OrderID: "b1", Event: "delete"}, {OrderID: "b1", Event: "add", LimitPrice: mustDecimal("99"), OrderQty: mustDecimal("1")}},
			nil,
		))

		Convey("the same order id re-added after delete lands with its new price", func() {
			owner.Fold("X/USD", func(view BookView) {
				So(len(view.Bids), ShouldEqual, 1)
				So(view.Bids[0].OrderID, ShouldEqual, "b1")
				So(view.Bids[0].LimitPrice.Cmp(mustDecimal("99")), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkBookOwnerUpdate(b *testing.B) {
	owner := NewBookOwner()
	owner.Apply(snapshot("X/USD",
		[]Level3Order{bid("b1", "100", "2"), bid("b2", "99", "5")},
		[]Level3Order{ask("a1", "101", "3"), ask("a2", "102", "7")},
	))
	mut := update("X/USD",
		[]Level3Order{{OrderID: "b2", Event: "modify", LimitPrice: mustDecimal("99.5"), OrderQty: mustDecimal("6")}},
		nil,
	)
	b.ReportAllocs()

	for b.Loop() {
		owner.Apply(mut)
	}
}
