package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/core"
	kmarket "github.com/theapemachine/symm/kraken/market"
)

func TestPublicClientConnect(t *testing.T) {
	convey.Convey("Given a websocket test server", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		publicClient := NewPublicClient(ctx, WithWebSocketURL(testServer.url))

		convey.Convey("It should connect when the dial target is reachable", func() {
			err := publicClient.Connect()
			convey.So(err, convey.ShouldBeNil)
			defer publicClient.Close()
		})
	})
}

func TestPublicClientPing(t *testing.T) {
	convey.Convey("Given a connected public client", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		publicClient := NewPublicClient(ctx, WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		convey.Convey("It should receive a pong after ping", func() {
			convey.So(publicClient.Ping(), convey.ShouldBeNil)

			payload, err := publicClient.Read()
			convey.So(err, convey.ShouldBeNil)

			var response struct {
				Method string `json:"method"`
			}
			convey.So(json.Unmarshal(payload, &response), convey.ShouldBeNil)
			convey.So(response.Method, convey.ShouldEqual, "pong")
		})
	})
}

func TestPublicClientSubscribeTo(t *testing.T) {
	convey.Convey("Given a connected public client", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		publicClient := NewPublicClient(ctx, WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		convey.Convey("It should subscribe to a public ticker channel", func() {
			params := kmarket.SubscribeParams{}.Ticker([]string{"BTC/EUR"})
			convey.So(publicClient.SubscribeTo(params), convey.ShouldBeNil)

			payload, err := publicClient.Read()
			convey.So(err, convey.ShouldBeNil)

			var response struct {
				Channel string `json:"channel"`
				Success bool   `json:"success"`
			}
			convey.So(json.Unmarshal(payload, &response), convey.ShouldBeNil)
			convey.So(response.Channel, convey.ShouldEqual, core.ChannelTicker)
			convey.So(response.Success, convey.ShouldBeTrue)
		})
	})
}

func TestPublicClientStartReader(t *testing.T) {
	convey.Convey("Given a connected public client with a reader loop", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		publicClient := NewPublicClient(ctx, WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		readDone := make(chan error, 1)
		go func() {
			readDone <- publicClient.StartReader(func(_ context.Context, payload []byte) error {
				var response struct {
					Method string `json:"method"`
				}
				if err := json.Unmarshal(payload, &response); err != nil {
					return err
				}
				if response.Method != "pong" {
					return nil
				}
				cancel()
				return nil
			})
		}()

		convey.Convey("It should stop when the handler cancels the session", func() {
			convey.So(publicClient.Ping(), convey.ShouldBeNil)
			convey.So(<-readDone, convey.ShouldBeNil)
		})
	})
}

func BenchmarkPublicClientSend(b *testing.B) {
	testServer := newTestWSServer(&testing.T{})
	defer testServer.Close()

	ctx := context.Background()
	publicClient := NewPublicClient(ctx, WithWebSocketURL(testServer.url))
	if err := publicClient.Connect(); err != nil {
		b.Fatalf("connect: %v", err)
	}
	defer publicClient.Close()

	go func() {
		for {
			if _, err := publicClient.Read(); err != nil {
				return
			}
		}
	}()

	message := map[string]any{"method": "ping"}

	for b.Loop() {
		if err := publicClient.Send(message); err != nil {
			b.Fatalf("send: %v", err)
		}
	}
}
