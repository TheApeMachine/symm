package paper

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestPairCatalogMeta(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a pair catalog", t, func() {
		catalog := NewPairCatalog(context.Background())

		Convey("It should parse base and quote assets", func() {
			So(catalog.baseAsset("BTC/EUR"), ShouldEqual, "BTC")
			So(catalog.quoteAsset("BTC/EUR"), ShouldEqual, "EUR")
		})

		Convey("It should return defaults for unknown symbols", func() {
			meta := catalog.Meta("UNKNOWN/PAIR")

			So(meta.takerPct, ShouldBeGreaterThan, 0)
			So(meta.tickSize, ShouldBeGreaterThan, 0)
			So(meta.quote, ShouldEqual, "PAIR")
		})
	})
}

func BenchmarkPairCatalogMeta(b *testing.B) {
	catalog := NewPairCatalog(context.Background())

	for b.Loop() {
		_ = catalog.Meta("BTC/EUR")
	}
}
