package broker_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskInitialize verifies the Desk subscribes to authoritative executions and
stays empty when Balance has no seeded holdings to adopt.
*/
func TestDeskInitialize(t *testing.T) {
	Convey("Given an empty Desk", t, func() {
		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		paper := websocket.NewPaper(
			ctx, websocket.NewLatencySimulator(system.NewBooter(ctx, nil)),
		)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		desk := broker.NewDesk(api, nil, nil, nil)

		err := desk.Initialize()

		Convey("It should become ready without inferring any positions", func() {
			So(err, ShouldBeNil)
			So(desk.Status(), ShouldEqual, types.READY)
			So(desk.OpenPositions(), ShouldEqual, 0)
		})
	})
}
