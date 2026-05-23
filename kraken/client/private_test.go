package client

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

func TestNewPrivateClient(t *testing.T) {
	convey.Convey("Given websocket credentials", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		convey.Convey("It should construct a private client with a public transport", func() {
			privateClient, err := NewPrivateClient(ctx, "public-key", "private-key")
			convey.So(err, convey.ShouldBeNil)
			convey.So(privateClient.conn, convey.ShouldNotBeNil)
		})
	})
}

func TestPrivateClientAuthenticate(t *testing.T) {
	convey.Convey("Given invalid API credentials", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		privateClient, err := NewPrivateClient(ctx, "invalid", "invalid")
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should record the token error without panicking", func() {
			convey.So(privateClient.Authenticate(), convey.ShouldNotBeNil)
			convey.So(privateClient.token.Err(), convey.ShouldNotBeNil)
		})
	})
}

func TestPrivateClientSubscribe(t *testing.T) {
	convey.Convey("Given a connected private client with a seeded token", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		privateClient, err := NewPrivateClient(
			ctx,
			"public-key",
			"private-key",
			WithWebSocketURL(testServer.url),
		)
		convey.So(err, convey.ShouldBeNil)
		convey.So(privateClient.conn.Connect(), convey.ShouldBeNil)
		defer privateClient.Close()

		privateClient.token = errnie.Does(func() (*kraken.Token, error) {
			token := &kraken.Token{}
			token.Result.Token = "session-token"
			token.Result.Expires = 9999999999
			return token, nil
		})

		convey.Convey("It should attach the token to authenticated subscriptions", func() {
			convey.So(privateClient.Subscribe(kraken.ChannelTypeExecutions), convey.ShouldBeNil)

			payload, readErr := privateClient.Read()
			convey.So(readErr, convey.ShouldBeNil)

			var response struct {
				Channel string `json:"channel"`
				Token   string `json:"token"`
				Success bool   `json:"success"`
			}
			convey.So(json.Unmarshal(payload, &response), convey.ShouldBeNil)
			convey.So(response.Channel, convey.ShouldEqual, string(kraken.ChannelTypeExecutions))
			convey.So(response.Token, convey.ShouldEqual, "session-token")
			convey.So(response.Success, convey.ShouldBeTrue)
		})
	})
}

func BenchmarkPrivateClientSubscribe(b *testing.B) {
	testServer := newTestWSServer(&testing.T{})
	defer testServer.Close()

	ctx := context.Background()
	privateClient, err := NewPrivateClient(
		ctx,
		"public-key",
		"private-key",
		WithWebSocketURL(testServer.url),
	)
	if err != nil {
		b.Fatalf("new private client: %v", err)
	}

	if err := privateClient.conn.Connect(); err != nil {
		b.Fatalf("connect: %v", err)
	}
	defer privateClient.Close()

	go func() {
		for {
			if _, readErr := privateClient.Read(); readErr != nil {
				return
			}
		}
	}()

	privateClient.token = errnie.Does(func() (*kraken.Token, error) {
		token := &kraken.Token{}
		token.Result.Token = "session-token"
		token.Result.Expires = 9999999999
		return token, nil
	})

	for b.Loop() {
		if err := privateClient.Subscribe(kraken.ChannelTypeExecutions); err != nil {
			b.Fatalf("subscribe: %v", err)
		}
	}
}
