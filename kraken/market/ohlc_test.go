package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestNewOHLC(t *testing.T) {
	Convey("Given a Kraken OHLC REST stub", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"last": 1717334400,
					"XXBTZEUR": [
						[1717334400, "65000.0", "65100.0", "64900.0", "65050.0", "65025.0", "12.5", 42]
					]
				}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		client := public.NewRest(ctx, public.EndpointType(server.URL))
		defer client.Close()

		ohlc, err := NewOHLC(ctx, client, "XXBTZEUR", 1, 0)

		Convey("It should decode the pagination cursor", func() {
			So(err, ShouldBeNil)
			So(ohlc.Last, ShouldEqual, 1717334400)
		})
	})
}
