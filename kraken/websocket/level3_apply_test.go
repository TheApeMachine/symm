package websocket

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestLiveUpdateLevel3DenseBookStaysBounded proves a quiet touch update against a
dense L3 book cannot pay math/big stringification across every resting order.
The SDK checksum path alone takes multiple milliseconds at this density.
*/
func TestLiveUpdateLevel3DenseBookStaysBounded(t *testing.T) {
	Convey("Given a dense authenticated L3 book", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		live.books.CreateBook("BTC/USD", 10)

		raw, err := denseLevel3Snapshot("BTC/USD", 80)
		So(err, ShouldBeNil)
		So(live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
			Data: kraken.NewWebSocketMessage(raw),
		}), ShouldBeNil)

		modify, err := denseLevel3TouchModify("BTC/USD", live, "2")
		So(err, ShouldBeNil)
		started := time.Now()
		So(live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
			Data: kraken.NewWebSocketMessage(modify),
		}), ShouldBeNil)
		elapsed := time.Since(started)

		Convey("Then a one-order update stays off the checksum cliff", func() {
			So(elapsed, ShouldBeLessThan, 500*time.Microsecond)
		})
	})
}

/*
TestLiveUpdateLevel3WireChecksumUsesTrailingZeroes proves checksum input keeps
Kraken's fixed-point wire text instead of a float round-trip.
*/
func TestLiveUpdateLevel3WireChecksumUsesTrailingZeroes(t *testing.T) {
	Convey("Given an L3 add with significant trailing zeroes", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		live.books.CreateBook("BTC/USD", 10)

		token := level3ChecksumToken("43125.300", "0.15000000")
		checksum := crc32.ChecksumIEEE([]byte(token))
		raw := []byte(fmt.Sprintf(`{
			"channel":"level3",
			"type":"snapshot",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%d,
				"bids":[{
					"event":"add",
					"order_id":"wire-1",
					"limit_price":43125.300,
					"order_qty":"0.15000000",
					"timestamp":"2026-07-12T00:00:00Z"
				}],
				"asks":[]
			}]
		}`, checksum))

		Convey("When the frame is applied", func() {
			err := live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
				Data: kraken.NewWebSocketMessage(raw),
			})

			Convey("Then wire-exact checksum validation succeeds", func() {
				So(err, ShouldBeNil)
				So(token, ShouldEqual, "4312530015000000")
			})
		})
	})
}

func BenchmarkLiveUpdateLevel3Dense(b *testing.B) {
	live := New(context.Background(), nil, true, Level3WebSocketURL)
	live.client.Reconnect = func() {}
	live.books.CreateBook("BTC/USD", 10)

	raw, err := denseLevel3Snapshot("BTC/USD", 80)

	if err != nil {
		b.Fatal(err)
	}

	if err := live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(raw),
	}); err != nil {
		b.Fatal(err)
	}

	modifyA, err := denseLevel3TouchModify("BTC/USD", live, "2")

	if err != nil {
		b.Fatal(err)
	}

	if err := live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
		Data: kraken.NewWebSocketMessage(modifyA),
	}); err != nil {
		b.Fatal(err)
	}

	modifyB, err := denseLevel3TouchModify("BTC/USD", live, "1")

	if err != nil {
		b.Fatal(err)
	}

	frames := [][]byte{modifyA, modifyB}
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		if err := live.updateLevel3(&callback.Event[*kraken.WebSocketMessage]{
			Data: kraken.NewWebSocketMessage(frames[index%2]),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func denseLevel3Snapshot(symbol string, ordersPerLevel int) ([]byte, error) {
	type row struct {
		event string
		id    string
		price string
		qty   string
		at    string
	}

	bids := make([]row, 0, 10*ordersPerLevel)
	asks := make([]row, 0, 10*ordersPerLevel)
	seq := 0

	for level := 0; level < 10; level++ {
		bidPrice := fmt.Sprintf("%d", 100000-level)
		askPrice := fmt.Sprintf("%d", 100001+level)

		for order := 0; order < ordersPerLevel; order++ {
			seq++
			at := fmt.Sprintf("2026-07-12T00:00:00.%09dZ", seq)
			bids = append(bids, row{
				event: "add",
				id:    fmt.Sprintf("bid-%d-%d", level, order),
				price: bidPrice,
				qty:   "1",
				at:    at,
			})
			seq++
			at = fmt.Sprintf("2026-07-12T00:00:00.%09dZ", seq)
			asks = append(asks, row{
				event: "add",
				id:    fmt.Sprintf("ask-%d-%d", level, order),
				price: askPrice,
				qty:   "1",
				at:    at,
			})
		}
	}

	var askTokens strings.Builder
	var bidTokens strings.Builder

	for level := 0; level < 10; level++ {
		for order := 0; order < ordersPerLevel; order++ {
			askTokens.WriteString(level3ChecksumToken(
				fmt.Sprintf("%d", 100001+level), "1",
			))
			bidTokens.WriteString(level3ChecksumToken(
				fmt.Sprintf("%d", 100000-level), "1",
			))
		}
	}

	checksum := crc32.ChecksumIEEE([]byte(askTokens.String() + bidTokens.String()))
	var body strings.Builder
	body.WriteString(`{"channel":"level3","type":"snapshot","data":[{`)
	body.WriteString(`"symbol":"`);
	body.WriteString(symbol);
	body.WriteString(`",`)
	fmt.Fprintf(&body, `"checksum":%d,`, checksum)
	body.WriteString(`"bids":[`)

	for index, bid := range bids {
		if index > 0 {
			body.WriteByte(',')
		}

		body.WriteString(fmt.Sprintf(
			`{"event":"%s","order_id":"%s","limit_price":%s,"order_qty":%s,"timestamp":"%s"}`,
			bid.event, bid.id, bid.price, bid.qty, bid.at,
		))
	}

	body.WriteString(`],"asks":[`)

	for index, ask := range asks {
		if index > 0 {
			body.WriteByte(',')
		}

		body.WriteString(fmt.Sprintf(
			`{"event":"%s","order_id":"%s","limit_price":%s,"order_qty":%s,"timestamp":"%s"}`,
			ask.event, ask.id, ask.price, ask.qty, ask.at,
		))
	}

	body.WriteString(`]}]}`)

	return []byte(body.String()), nil
}

func denseLevel3TouchModify(
	symbol string,
	live *Live,
	qty string,
) ([]byte, error) {
	managed := live.books.GetBook(symbol)

	if managed == nil {
		return nil, fmt.Errorf("missing book")
	}

	var askTokens strings.Builder
	var bidTokens strings.Builder
	cursor := managed.BestAsk()

	for range 10 {
		if cursor == nil {
			break
		}

		for _, order := range cursor.Queue() {
			wire, ok := live.level3.wire(symbol, order.ID)

			if !ok {
				return nil, fmt.Errorf("missing ask wire %s", order.ID)
			}

			askTokens.WriteString(wire.token)
		}

		cursor = cursor.Higher
	}

	cursor = managed.BestBid()

	for range 10 {
		if cursor == nil {
			break
		}

		for _, order := range cursor.Queue() {
			if order.ID == "bid-0-0" {
				bidTokens.WriteString(level3ChecksumToken("100000", qty))
				continue
			}

			wire, ok := live.level3.wire(symbol, order.ID)

			if !ok {
				return nil, fmt.Errorf("missing bid wire %s", order.ID)
			}

			bidTokens.WriteString(wire.token)
		}

		cursor = cursor.Lower
	}

	checksum := crc32.ChecksumIEEE([]byte(askTokens.String() + bidTokens.String()))

	// Keep the original timestamp so queue order (time priority) is unchanged
	// and the predicted checksum matches the post-apply walk.
	return []byte(fmt.Sprintf(`{
		"channel":"level3",
		"type":"update",
		"data":[{
			"symbol":"%s",
			"checksum":%d,
			"bids":[{
				"event":"modify",
				"order_id":"bid-0-0",
				"limit_price":100000,
				"order_qty":%s,
				"timestamp":"2026-07-12T00:00:00.000000001Z"
			}],
			"asks":[]
		}]
	}`, symbol, checksum, qty)), nil
}
