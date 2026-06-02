package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestAssetPairsPairByWsname(t *testing.T) {
	Convey("Given asset pair metadata keyed by internal name", t, func() {
		pairs := AssetPairs{
			"XXBTZEUR": {Wsname: "BTC/EUR", Quote: "ZEUR"},
			"XETHZEUR": {Wsname: "ETH/EUR", Quote: "ZEUR"},
		}

		Convey("It should resolve the WebSocket symbol", func() {
			pair, err := pairs.PairByWsname("BTC/EUR")

			So(err, ShouldBeNil)
			So(pair, ShouldNotBeNil)
			So(pair.Quote, ShouldEqual, "ZEUR")
		})

		Convey("It should error when the wsname is absent", func() {
			pair, err := pairs.PairByWsname("DOGE/EUR")

			So(err, ShouldNotBeNil)
			So(pair, ShouldBeNil)
		})
	})
}

func TestNewAssetPairs(t *testing.T) {
	Convey("Given a Kraken AssetPairs REST stub", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZEUR": {"wsname":"BTC/EUR","quote":"ZEUR","base":"XXBT"}
				}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		client := public.NewRest(ctx, public.EndpointType(server.URL))
		defer client.Close()

		pairs, err := NewAssetPairs(ctx, client)

		Convey("It should decode tradable pair metadata", func() {
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 1)

			pair, lookupErr := pairs.PairByWsname("BTC/EUR")

			So(lookupErr, ShouldBeNil)
			So(pair.Base, ShouldEqual, "XXBT")
		})
	})
}
