package futures

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestCatalogProductsForSpotPair(t *testing.T) {
	convey.Convey("Given a futures instruments catalog", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"result":"success",
				"instruments":[
					{"symbol":"PI_XBTUSD","tradeable":true,"isExpired":false},
					{"symbol":"FI_XBTUSD_260626","tradeable":true,"isExpired":false},
					{"symbol":"PI_ETHUSD","tradeable":true,"isExpired":false},
					{"symbol":"FI_ETHUSD_260626","tradeable":true,"isExpired":false}
				]
			}`))
		}))
		defer server.Close()

		catalog := &Catalog{
			client: server.Client(),
		}

		convey.Convey("It should map spot pairs to perpetual and dated products by base", func() {
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			convey.So(err, convey.ShouldBeNil)

			response, err := catalog.client.Do(request)
			convey.So(err, convey.ShouldBeNil)

			defer response.Body.Close()

			payload, readErr := io.ReadAll(response.Body)
			convey.So(readErr, convey.ShouldBeNil)

			byBase, parseErr := parseInstrumentPayload(payload)
			convey.So(parseErr, convey.ShouldBeNil)

			catalog.loaded.Store(true)
			catalog.state.Store(&catalogState{byBase: byBase})

			products, err := catalog.ProductsForSpotPair("XBT/USD")
			convey.So(err, convey.ShouldBeNil)
			convey.So(products, convey.ShouldContain, "PI_XBTUSD")
			convey.So(products, convey.ShouldContain, "FI_XBTUSD_260626")
			convey.So(products, convey.ShouldNotContain, "PI_ETHUSD")
		})

		convey.Convey("It should return no products without error when the base has no derivatives", func() {
			catalog.loaded.Store(true)
			catalog.state.Store(&catalogState{byBase: make(map[string][]string)})

			products, err := catalog.ProductsForSpotPair("DOGE/USD")
			convey.So(err, convey.ShouldBeNil)
			convey.So(products, convey.ShouldBeEmpty)
		})
	})
}
