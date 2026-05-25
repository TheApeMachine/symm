package client

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewPrivateClient(t *testing.T) {
	convey.Convey("Given API credentials", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		convey.Convey("It should construct a private client", func() {
			privateClient := NewPrivateClient(ctx, pool, "wss://ws-auth.kraken.com/v2", "key", "secret")
			convey.So(privateClient, convey.ShouldNotBeNil)
		})
	})
}

func TestPrivateClientConnectRequiresCredentials(t *testing.T) {
	convey.Convey("Given a private client without credentials", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		privateClient := NewPrivateClient(ctx, pool, testServer.url, "", "")

		convey.Convey("It should reject connect", func() {
			convey.So(privateClient.Connect(), convey.ShouldNotBeNil)
		})
	})
}
