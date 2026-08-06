package websocket

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	sdk "github.com/theapemachine/api-go/v2/pkg/kraken"
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

		Convey("All should return a detached snapshot", func() {
			value, found := managed.All().Load("BTC/USD")
			So(found, ShouldBeTrue)

			snapshot := value.(*spotbook.Book)
			snapshot.Bids.Levels = map[string]*spotbook.Level{}

			current := managed.Get("BTC/USD")
			So(current, ShouldNotBeNil)
			So(len(current.Bids.Levels), ShouldEqual, 32)
		})

		Convey("Snapshots should remain race-free while updates continue", func() {
			waitGroup := sync.WaitGroup{}
			waitGroup.Add(8)

			for range 8 {
				go func() {
					defer waitGroup.Done()

					for range 256 {
						managed.All().Range(func(key, value any) bool {
							snapshot := value.(*spotbook.Book)
							_ = snapshot.BestBid()
							return true
						})
					}
				}()
			}

			for index := range 256 {
				price := decimal.NewFromInt64(int64(100 + index%32))
				payload := &kraken.Level3{Data: []kraken.Level3Data{{
					Symbol: "BTC/USD",
					Bids: []kraken.Level3Order{{
						OrderID:    fmt.Sprintf("bid-%d", index%32),
						LimitPrice: price,
						OrderQty:   decimal.NewFromInt64(int64(index%7 + 1)),
						Timestamp:  time.Now().UTC(),
					}},
				}}}

				So(managed.Update(event, payload), ShouldBeNil)
			}

			waitGroup.Wait()
		})

		Convey("One snapshot should support concurrent Level3 queue reads", func() {
			snapshot := managed.Get("BTC/USD")
			orders := 0

			for _, level := range snapshot.Bids.Levels {
				orders += len(level.Queue())
			}

			So(orders, ShouldEqual, 32)

			waitGroup := sync.WaitGroup{}
			waitGroup.Add(8)

			for range 8 {
				go func() {
					defer waitGroup.Done()

					for range 256 {
						for _, level := range snapshot.Bids.Levels {
							_ = level.Queue()
						}
					}
				}()
			}

			waitGroup.Wait()
		})
	})
}

func TestBookUpdate(t *testing.T) {
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
			book := managed.Get("BTC/USD")
			So(book.Bids.Levels, ShouldHaveLength, 1)
			So(book.BestBid().Quantity.Float64(), ShouldEqual, 2.0)

			So(managed.Update(event, deletion("bid-two")), ShouldBeNil)
			book = managed.Get("BTC/USD")
			So(book.Bids.Levels, ShouldBeEmpty)
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
