package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGrammarCatalogSymbol(t *testing.T) {
	Convey("The symbol is read from the object key", t, func() {
		catalog := &GrammarCatalog{}

		So(catalog.Symbol(grammarSymbolDir+"BTC-USD.bin"), ShouldEqual, "BTC/USD")
		So(catalog.ObjectKey("ETH/USD"), ShouldEqual, grammarSymbolDir+"ETH-USD.bin")
	})
}
