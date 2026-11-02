package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckEntryPhysicalRejectsNilPrice(t *testing.T) {
	Convey("entry is blocked when the book cannot be priced", t, func() {
		decision := CheckEntryPhysical(nil, "Q/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "no price book")
	})
}

func TestCheckEntryPhysicalRejectsThinBids(t *testing.T) {
	Convey("entry is blocked when top bids have no cash depth", t, func() {
		price := &Price{
			Books: NewBook(context.Background(), spot.NewNormalizer()),
		}
		decision := CheckEntryPhysical(price, "THIN/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "book too thin")
	})
}

