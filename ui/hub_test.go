package ui

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewHub(t *testing.T) {
	Convey("Given the dashboard thesis", t, func() {
		channel := make(chan []byte, 1)
		thesis := types.NewThesis(channel)

		hub, err := NewHub(context.Background(), nil, nil, thesis, channel)

		Convey("It should retain the thesis used by websocket publication", func() {
			So(err, ShouldBeNil)
			So(hub.thesis, ShouldEqual, thesis)
		})
	})
}
