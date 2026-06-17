package futures

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCatalogProductsForSpotPair(testingTB *testing.T) {
	Convey("Given the shared futures catalog", testingTB, func() {
		catalog := SharedCatalog()

		So(catalog.EnsureLoaded(context.Background()), ShouldBeNil)

		Convey("It should return linked products for a known spot pair", func() {
			products, err := catalog.ProductsForSpotPair("XBT/USD")

			So(err, ShouldBeNil)
			So(products, ShouldResemble, []string{"PI_XBTUSD"})
		})

		Convey("It should reject unknown spot pairs", func() {
			_, err := catalog.ProductsForSpotPair("NOPE/USD")

			So(err, ShouldNotBeNil)
		})
	})
}
