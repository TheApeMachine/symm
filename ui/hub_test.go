package ui

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func TestNewHub(t *testing.T) {
	Convey("Given a ui hub with an enlarged read buffer", t, func() {
		viper.Set("ui.addr", "127.0.0.1:0")
		viper.Set("ui.read_buffer_size", 16*1024)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		defer pool.Close()

		hub := NewHub(ctx, pool)

		So(hub, ShouldNotBeNil)
		So(hub.app.Config().ReadBufferSize, ShouldEqual, 16*1024)

		t.Cleanup(func() {
			_ = hub.Close()
		})
	})
}
