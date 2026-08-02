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
			done := make(chan struct{})

			go func() {
				defer close(done)

				for range 256 {
					managed.All().Range(func(key, value any) bool {
						snapshot := value.(*spotbook.Book)
						_ = snapshot.BestBid()
						return true
					})
				}
			}()

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

			<-done
		})
	})
}
