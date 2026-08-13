package websocket

import (
	"fmt"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestBookAll(t *testing.T) {
	Convey("Given a reconstructed Level3 book", t, func() {
		managed := NewBook(t.Context())
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
	Convey("Given a Level 3 cache connected to its owning transport", t, func() {
		managed := NewBook(t.Context())
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
		managed := NewBook(t.Context())
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
		managed := NewBook(t.Context())
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

	Convey("Given a venue frame whose intermediate state is crossed", t, func() {
		managed := NewBook(t.Context())
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
	managed := NewBook(b.Context())
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
	managed := NewBook(b.Context())
	managed.Create("BTC/USD", 32)
	read := func(*spotbook.Book) {}
	b.ReportAllocs()

	for b.Loop() {
		managed.Get("BTC/USD", read)
	}
}

func BenchmarkBookUpdate(b *testing.B) {
	managed := NewBook(b.Context())
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
