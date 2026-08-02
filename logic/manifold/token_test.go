package manifold

import (
	"testing"
	"time"

	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestTokenizerNewBatch(t *testing.T) {
	Convey("Given a tokenizer with a deterministic market universe", t, func() {
		price := decimal.NewFromInt64(100)
		quantity := decimal.NewFromInt64(1)
		orders := []*mgrbook.Order{{
			ID:         "order",
			LimitPrice: price,
			Quantity:   quantity,
			Timestamp:  time.Unix(1, 0).UTC(),
		}}
		tokenizer := NewTokenizer(pfluid.DefaultConfig(), []string{"ETH/USD", "BTC/USD"})

		_, bitcoinContent, bitcoinErr := tokenizer.NewBatch(
			orders, nil, 100, 1, 1, "BTC/USD",
		)
		_, etherContent, etherErr := tokenizer.NewBatch(
			orders, nil, 100, 1, 1, "ETH/USD",
		)

		Convey("It should assign distinct stable content identities by symbol", func() {
			So(bitcoinErr, ShouldBeNil)
			So(etherErr, ShouldBeNil)
			So(bitcoinContent, ShouldResemble, []uint32{0})
			So(etherContent, ShouldResemble, []uint32{2})
		})

		Convey("It should reject a symbol outside that universe", func() {
			_, _, err := tokenizer.NewBatch(
				orders, nil, 100, 1, 1, "SOL/USD",
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "SOL/USD")
		})
	})
}
