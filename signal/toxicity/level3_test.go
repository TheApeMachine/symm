package toxicity

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/websocket"
)

func TestNewLevel3(t *testing.T) {
	Convey("Given the shared Kraken API", t, func() {
		level3 := NewLevel3(
			websocket.NewAPI(context.Background(), nil, nil, nil),
		)

		Convey("Then toxicity consumes its SDK book managers directly", func() {
			So(level3, ShouldNotBeNil)
		})
	})
}
