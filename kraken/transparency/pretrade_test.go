package transparency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNewPreTrade(t *testing.T) {
	Convey("Given a Kraken pre-trade transparency REST stub", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"symbol": "BTC/EUR",
					"description": "Bitcoin / Euro",
					"base_asset": "BTC",
					"quote_asset": "EUR",
					"venue": "KRAKEN",
					"system": "KRAKEN"
				}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		client := public.NewRest(ctx, public.EndpointType(server.URL))
		defer client.Close()

		pretrade, err := NewPreTrade(ctx, client, []string{"BTC/EUR"})

		Convey("It should decode instrument transparency metadata", func() {
			So(err, ShouldBeNil)
			So(pretrade.Symbol, ShouldEqual, "BTC/EUR")
			So(pretrade.BaseAsset, ShouldEqual, "BTC")
			So(pretrade.QuoteAsset, ShouldEqual, "EUR")
			So(pretrade.Venue, ShouldEqual, "KRAKEN")
		})
	})
}
