package websocket

import (
	"fmt"
	"strings"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func newBookFixture(
	testingTB testing.TB,
	symbol string,
	pricePrecision int,
	quantityPrecision int,
) *Book {
	testingTB.Helper()
	base, quote, found := strings.Cut(symbol, "/")

	if !found {
		testingTB.Fatalf("book fixture pair required: %s", symbol)
	}

	normalizer := spot.NewNormalizer()
	tickSizeText := "1"

	if pricePrecision > 0 {
		tickSizeText = "0." + strings.Repeat("0", pricePrecision-1) + "1"
	}

	tickSize, err := decimal.NewFromString(tickSizeText)

	if err != nil {
		testingTB.Fatalf("book fixture tick size: %v", err)
	}

	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			base:  {AltName: base},
			quote: {AltName: quote},
		},
		NewPairs: map[string]spot.AssetPair{
			base + quote: {
				WSName: symbol, Base: base, Quote: quote,
				PairDecimals: pricePrecision, LotDecimals: quantityPrecision,
				LotMultiplier: 1, TickSize: tickSize,
			},
		},
	})

	return NewBook(testingTB.Context(), normalizer)
}

func TestBookAll(t *testing.T) {
	Convey("Given a reconstructed Level3 book", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}

		for index := range 32 {
			price := decimal.NewFromInt64(int64(100 + index))
			payload := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{{
					OrderID:    fmt.Sprintf("bid-%d", index),
					LimitPrice: price,
					OrderQty:   decimal.NewFromInt64(1),
					Timestamp:  time.Now().UTC(),
				}},
			}}}

			So(managed.Update(event, payload), ShouldBeNil)
		}

		Convey("All should expose the managed book", func() {
			value, found := managed.All().Load("BTC/USD")
			So(found, ShouldBeTrue)
			managed.Get("BTC/USD", func(current *spotbook.Book) {
				So(value.(*spotbook.Book), ShouldEqual, current)
			})
		})
	})
}

func TestBookUpdate(t *testing.T) {
	Convey("Given authoritative fixed-point Level 3 decimals", t, func() {
		managed := newBookFixture(t, "CELR/USD", 7, 5)
		managed.Create("CELR/USD", 10)
		askPrice, err := decimal.NewFromString("0.00035")
		So(err, ShouldBeNil)
		askQuantity, err := decimal.NewFromString("1234.5")
		So(err, ShouldBeNil)
		bidPrice, err := decimal.NewFromString("0.00034")
		So(err, ShouldBeNil)
		bidQuantity, err := decimal.NewFromString("2000")
		So(err, ShouldBeNil)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		payload := &kraken.Level3{
			Type: "snapshot",
			Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 3152022922,
				Bids: []kraken.Level3Order{{
					OrderID: "bid", LimitPrice: bidPrice,
					OrderQty: bidQuantity, Timestamp: time.Unix(1, 0).UTC(),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask", LimitPrice: askPrice,
					OrderQty: askQuantity, Timestamp: time.Unix(1, 0).UTC(),
				}},
			}},
		}

		Convey("It should restore venue precision before checksum validation", func() {
			So(managed.Update(event, payload), ShouldBeNil)
			managed.Get("CELR/USD", func(book *spotbook.Book) {
				So(book.BestAsk().Price.String(), ShouldEqual, "0.0003500")
				So(book.BestAsk().Quantity.String(), ShouldEqual, "1234.50000")
				So(book.BestBid().Price.String(), ShouldEqual, "0.0003400")
				So(book.BestBid().Quantity.String(), ShouldEqual, "2000.00000")
			})
		})
	})

	Convey("Given a Level 3 cache connected to its owning transport", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		updates := make(chan string, 1)
		managed.SetUpdates(updates)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}

		err := managed.Update(event, payload)

		Convey("It should publish the symbol only after applying the update", func() {
			So(err, ShouldBeNil)
			So(<-updates, ShouldEqual, "BTC/USD")
			managed.Get("BTC/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given backpressure from the Level3 event consumer", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		updates := make(chan string, 1)
		events := make(chan kraken.Level3Data)
		managed.SetUpdates(updates)
		managed.SetEvents(events)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}
		updateDone := make(chan error, 1)

		go func() {
			updateDone <- managed.Update(event, payload)
		}()

		So(<-updates, ShouldEqual, "BTC/USD")
		readDone := make(chan struct{})
		bestBidFound := false

		go func() {
			managed.Get("BTC/USD", func(book *spotbook.Book) {
				bestBidFound = book.BestBid() != nil
			})
			close(readDone)
		}()

		readBeforeDrain := false

		select {
		case <-readDone:
			readBeforeDrain = true
		case <-time.After(100 * time.Millisecond):
		}

		accepted := <-events

		if !readBeforeDrain {
			<-readDone
		}

		Convey("It should release the book before waiting for the consumer", func() {
			So(readBeforeDrain, ShouldBeTrue)
			So(bestBidFound, ShouldBeTrue)
			So(accepted.Symbol, ShouldEqual, "BTC/USD")
			So(<-updateDone, ShouldBeNil)
		})
	})

	Convey("Given two Level3 orders at one price", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		price := decimal.NewFromInt64(100)
		at := time.Unix(1_700_008_000, 0).UTC()
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{
				{
					Event: "add", OrderID: "bid-one", LimitPrice: price,
					OrderQty: decimal.NewFromInt64(3), Timestamp: at,
				},
				{
					Event: "add", OrderID: "bid-two", LimitPrice: price,
					OrderQty: decimal.NewFromInt64(2), Timestamp: at,
				},
			},
		}}}
		So(managed.Update(event, payload), ShouldBeNil)

		Convey("Deleting orders should remove their actual queued quantity", func() {
			deletion := func(orderID string) *kraken.Level3 {
				return &kraken.Level3{Data: []kraken.Level3Data{{
					Symbol: "BTC/USD",
					Bids: []kraken.Level3Order{{
						Event: "delete", OrderID: orderID,
						LimitPrice: price, Timestamp: at.Add(time.Second),
					}},
				}}}
			}

			So(managed.Update(event, deletion("bid-one")), ShouldBeNil)
			managed.Get("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldHaveLength, 1)
				So(book.BestBid().Quantity.Float64(), ShouldEqual, 2.0)
			})

			So(managed.Update(event, deletion("bid-two")), ShouldBeNil)
			managed.Get("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a snapshot and updates whose checksums disagree with local state", t, func() {
		managed := newBookFixture(t, "CELR/USD", 7, 5)
		managed.Create("CELR/USD", 10)
		resynced := make(chan string, 4)
		managed.SetResync(func(symbol string) { resynced <- symbol })
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		knownGood := &kraken.Level3{
			Type: "snapshot",
			Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 3152022922,
				Bids: []kraken.Level3Order{{
					OrderID: "bid", LimitPrice: decimal.NewFromFloat64(0.00034),
					OrderQty: decimal.NewFromInt64(2000), Timestamp: time.Unix(1, 0).UTC(),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask", LimitPrice: decimal.NewFromFloat64(0.00035),
					OrderQty: decimal.NewFromFloat64(1234.5), Timestamp: time.Unix(1, 0).UTC(),
				}},
			}},
		}

		Convey("It should diverge once, drop further deltas, and recover on the resubscription snapshot", func() {
			So(managed.Update(event, knownGood), ShouldBeNil)

			// A wrong server checksum for state that is known good: local
			// state is untrustworthy from here on.
			diverged := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 1,
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "late-bid",
					LimitPrice: decimal.NewFromFloat64(0.00033),
					OrderQty:   decimal.NewFromInt64(5), Timestamp: time.Unix(2, 0).UTC(),
				}},
			}}}

			So(managed.Update(event, diverged), ShouldNotBeNil)
			So(<-resynced, ShouldEqual, "CELR/USD")
			managed.Get("CELR/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})

			// Deltas for a diverged symbol are dropped without touching the
			// empty book and without asking for a second resubscription.
			So(managed.Update(event, diverged), ShouldBeNil)

			select {
			case symbol := <-resynced:
				So(symbol, ShouldBeNil)
			default:
			}

			managed.Get("CELR/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})

			// The resubscription snapshot is authoritative again.
			So(managed.Update(event, knownGood), ShouldBeNil)
			managed.Get("CELR/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
				So(book.BestAsk(), ShouldNotBeNil)
			})

			accepted := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "CELR/USD",
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "fresh-bid",
					LimitPrice: decimal.NewFromFloat64(0.00033),
					OrderQty:   decimal.NewFromInt64(5), Timestamp: time.Unix(3, 0).UTC(),
				}},
			}}}

			So(managed.Update(event, accepted), ShouldBeNil)
			managed.Get("CELR/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given a venue frame whose intermediate state is crossed", t, func() {
		managed := newBookFixture(t, "APR/USD", 0, 0)
		managed.Create("APR/USD", 10)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		initial := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "APR/USD",
			Asks: []kraken.Level3Order{
				{
					Event: "add", OrderID: "consumed", LimitPrice: decimal.NewFromInt64(99),
					OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
				},
				{
					Event: "add", OrderID: "resting", LimitPrice: decimal.NewFromInt64(100),
					OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(3, 0).UTC(),
				},
			},
		}}}
		So(managed.Update(event, initial), ShouldBeNil)
		update := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "APR/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "new-bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(2, 0).UTC(),
			}},
			Asks: []kraken.Level3Order{{
				Event: "delete", OrderID: "consumed", LimitPrice: decimal.NewFromInt64(99),
				Timestamp: time.Unix(2, 1).UTC(),
			}},
		}}}

		err := managed.Update(event, update)

		Convey("It should retain the venue's final locked book for checksum validation", func() {
			So(err, ShouldBeNil)
			managed.Get("APR/USD", func(book *spotbook.Book) {
				So(book.NoBookCrossing, ShouldBeFalse)
				So(book.BestBid(), ShouldNotBeNil)
				So(book.BestAsk(), ShouldNotBeNil)
				So(book.BestBid().Price.Cmp(book.BestAsk().Price), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkBookAll(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 256)
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
	}

	for index := range 256 {
		price := decimal.NewFromInt64(int64(100 + index))
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				OrderID:    fmt.Sprintf("bid-%d", index),
				LimitPrice: price,
				OrderQty:   decimal.NewFromInt64(1),
				Timestamp:  time.Unix(int64(index), 0).UTC(),
			}},
		}}}

		if err := managed.Update(event, payload); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		managed.All()
	}
}

func BenchmarkBookGet(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 32)
	read := func(*spotbook.Book) {}
	b.ReportAllocs()

	for b.Loop() {
		managed.Get("BTC/USD", read)
	}
}

func BenchmarkBookUpdate(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 32)
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
	}
	price := decimal.NewFromInt64(100)
	quantity := decimal.NewFromInt64(1)
	at := time.Unix(1_700_008_000, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		_ = managed.Update(event, &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid-one", LimitPrice: price,
				OrderQty: quantity, Timestamp: at,
			}},
		}}})
		_ = managed.Update(event, &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "delete", OrderID: "bid-one", LimitPrice: price,
				Timestamp: at.Add(time.Second),
			}},
		}}})
	}
}
