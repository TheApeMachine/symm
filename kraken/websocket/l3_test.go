package websocket

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"

	. "github.com/smartystreets/goconvey/convey"
)

func TestL3Subscribe(t *testing.T) {
	Convey("Given an unauthenticated level3 websocket", t, func() {
		l3 := &L3{
			client: spot.NewWebSocket(),
			depth:  10,
		}

		Convey("When symbols are registered before authentication", func() {
			l3.Subscribe([]string{"BTC/USD"})

			Convey("It should retain symbols without subscribing yet", func() {
				So(l3.symbols, ShouldResemble, []string{"BTC/USD"})
				So(l3.subscribed, ShouldBeFalse)
			})
		})
	})
}
