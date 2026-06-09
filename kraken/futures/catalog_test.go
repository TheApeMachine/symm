package futures

import (
	"context"
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
			byBase: make(map[string][]string),
		}

		convey.Convey("It should map spot pairs to perpetual and dated products by base", func() {
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			convey.So(err, convey.ShouldBeNil)

			response, err := catalog.client.Do(request)
			convey.So(err, convey.ShouldBeNil)

			defer response.Body.Close()

			payload := make([]byte, 0, 4096)
			buffer := make([]byte, 4096)

			for {
				count, readErr := response.Body.Read(buffer)

				if count > 0 {
					payload = append(payload, buffer[:count]...)
				}

				if readErr != nil {
					break
				}
			}

			convey.So(catalog.parseInstruments(payload), convey.ShouldBeNil)

			products, err := catalog.ProductsForSpotPair("XBT/USD")
			convey.So(err, convey.ShouldBeNil)
			convey.So(products, convey.ShouldContain, "PI_XBTUSD")
			convey.So(products, convey.ShouldContain, "FI_XBTUSD_260626")
			convey.So(products, convey.ShouldNotContain, "PI_ETHUSD")
		})

		convey.Convey("It should return no products without error when the base has no derivatives", func() {
			products, err := catalog.ProductsForSpotPair("DOGE/USD")
			convey.So(err, convey.ShouldBeNil)
			convey.So(products, convey.ShouldBeEmpty)
		})
	})
}
