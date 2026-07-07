package ui

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func findFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")

	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)

	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
}

func TestHub(t *testing.T) {
	Convey("Given a new Hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := findFreePort()
		So(err, ShouldBeNil)

		viper.Set("ui.addr", addr)
		viper.Set("system.websocket.channel.buffer", 10)

		hub, err := NewHub(ctx)
		So(err, ShouldBeNil)

		go func() {
			_ = hub.Serve()
		}()

		// Wait for server to start
		time.Sleep(100 * time.Millisecond)

		Convey("When multiple clients connect", func() {
			url := "ws://" + addr + "/ws"
			conn1, _, err := websocket.DefaultDialer.Dial(url, nil)
			So(err, ShouldBeNil)
			defer conn1.Close()

			conn2, _, err := websocket.DefaultDialer.Dial(url, nil)
			So(err, ShouldBeNil)
			defer conn2.Close()

			Convey("And a message is broadcast", func() {
				msg := []byte(`{"event":"ticker","symbol":"BTC/USD","data":"test"}`)
				hub.Messages <- msg

				Convey("Both clients should receive the message", func() {
					_, res1, err := conn1.ReadMessage()
					So(err, ShouldBeNil)
					So(string(res1), ShouldEqual, string(msg))

					_, res2, err := conn2.ReadMessage()
					So(err, ShouldBeNil)
					So(string(res2), ShouldEqual, string(msg))
				})
			})
		})

		Convey("When a client disconnects", func() {
			url := "ws://" + addr + "/ws"
			conn1, _, err := websocket.DefaultDialer.Dial(url, nil)
			So(err, ShouldBeNil)

			conn2, _, err := websocket.DefaultDialer.Dial(url, nil)
			So(err, ShouldBeNil)
			defer conn2.Close()

			// Close conn1 immediately
			err = conn1.Close()
			So(err, ShouldBeNil)

			// Wait a bit to let the server detect disconnect and delete the client
			time.Sleep(100 * time.Millisecond)

			Convey("Other clients should still receive messages", func() {
				msg := []byte(`{"event":"ticker","symbol":"BTC/USD","data":"test"}`)
				hub.Messages <- msg

				_, res2, err := conn2.ReadMessage()
				So(err, ShouldBeNil)
				So(string(res2), ShouldEqual, string(msg))
			})
		})

		Convey("When a message matching cached entity is received", func() {
			msg := []byte(`{"balances":[{"asset":"USD","free":100}]}`)
			hub.Messages <- msg

			// Wait for broadcast to cache the message
			time.Sleep(50 * time.Millisecond)

			Convey("A new client connecting should receive the cached message", func() {
				url := "ws://" + addr + "/ws"
				conn, _, err := websocket.DefaultDialer.Dial(url, nil)
				So(err, ShouldBeNil)
				defer conn.Close()

				_, res, err := conn.ReadMessage()
				So(err, ShouldBeNil)
				So(string(res), ShouldContainSubstring, "balances")
			})
		})
	})
}

func BenchmarkHubBroadcast(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := findFreePort()

	if err != nil {
		b.Fatal(err)
	}

	viper.Set("ui.addr", addr)
	viper.Set("system.websocket.channel.buffer", 1000)

	hub, err := NewHub(ctx)

	if err != nil {
		b.Fatal(err)
	}

	go func() {
		_ = hub.Serve()
	}()

	time.Sleep(100 * time.Millisecond)

	url := "ws://" + addr + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)

	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	msg := []byte(`{"event":"ticker","symbol":"BTC/USD"}`)

	b.ResetTimer()
	b.ReportAllocs()

	// Drain goroutine to consume messages in background
	go func() {
		for {
			_, _, err := conn.ReadMessage()

			if err != nil {
				return
			}
		}
	}()

	for b.Loop() {
		hub.Messages <- msg
	}
}
