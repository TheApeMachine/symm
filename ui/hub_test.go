package ui

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	krakenws "github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
)

type hubStubConn struct {
	channels map[string][]func([]byte)
}

func (stub *hubStubConn) Client() *spot.WebSocket { return nil }

func (stub *hubStubConn) Books() *spot.BookManager { return nil }

func (stub *hubStubConn) On(channel string, action func([]byte)) {
	if stub.channels == nil {
		stub.channels = map[string][]func([]byte){}
	}

	stub.channels[channel] = append(stub.channels[channel], action)
}

func (stub *hubStubConn) Write(params json.Marshaler) error { return nil }

func (stub *hubStubConn) Close() {}

func (stub *hubStubConn) Post(path string, params json.Marshaler) ([]byte, error) {
	return nil, nil
}

func testHubDeps(ui chan []byte) (*broker.Price, *broker.Balance, *broker.Desk) {
	api := krakenws.NewAPI(
		&hubStubConn{},
		&hubStubConn{},
		&hubStubConn{},
		krakenws.NewPaper(context.Background(),
			krakenws.NewLatencySimulator(system.NewBooter(context.Background(), ui)),
		),
	)

	price := broker.NewPrice(api, ui)
	balance := broker.NewBalance(api, ui)
	desk := broker.NewDesk(api, price, balance, ui)

	return price, balance, desk
}

func findFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")

	if err != nil {
		return "", err
	}

	listener, err := net.ListenTCP("tcp", addr)

	if err != nil {
		return "", err
	}

	defer listener.Close()
	return listener.Addr().String(), nil
}

func startTestHub(t *testing.T) (context.Context, context.CancelFunc, *Hub, string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	addr, err := findFreePort()
	So(err, ShouldBeNil)

	viper.Set("ui.addr", addr)
	viper.Set("system.websocket.channel.buffer", 10)

	uiChannel := make(chan []byte, 10)
	price, balance, desk := testHubDeps(uiChannel)
	hub, err := NewHub(ctx, price, balance, desk, uiChannel)
	So(err, ShouldBeNil)

	go func() {
		_ = hub.Serve()
	}()

	time.Sleep(100 * time.Millisecond)

	return ctx, cancel, hub, addr
}

func awaitMessage(conn *websocket.Conn, want []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(deadline)

		_, res, err := conn.ReadMessage()

		if err != nil {
			return err
		}

		if string(res) == string(want) {
			return nil
		}
	}

	return context.DeadlineExceeded
}

func TestHub(t *testing.T) {
	Convey("Given a hub with one connected client", t, func() {
		_, cancel, hub, addr := startTestHub(t)
		defer cancel()
		defer hub.Close()

		url := "ws://" + addr + "/ws"
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		So(err, ShouldBeNil)
		defer conn.Close()

		time.Sleep(50 * time.Millisecond)

		Convey("When a message is published", func() {
			msg := []byte(`{"event":"ticker","symbol":"BTC/USD","data":"test"}`)
			hub.Messages <- msg

			Convey("The client should receive the message", func() {
				err := awaitMessage(conn, msg, 2*time.Second)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestHubStopsPublishingAfterClientDisconnects(t *testing.T) {
	Convey("Given a hub whose client disconnects, as on a page refresh", t, func() {
		_, cancel, hub, addr := startTestHub(t)
		defer cancel()
		defer hub.Close()

		url := "ws://" + addr + "/ws"
		firstConn, _, err := websocket.DefaultDialer.Dial(url, nil)
		So(err, ShouldBeNil)

		time.Sleep(50 * time.Millisecond)
		firstConn.Close()
		time.Sleep(50 * time.Millisecond)

		secondConn, _, err := websocket.DefaultDialer.Dial(url, nil)
		So(err, ShouldBeNil)
		defer secondConn.Close()

		time.Sleep(50 * time.Millisecond)

		Convey("The reconnected client should still receive published messages", func() {
			msg := []byte(`{"event":"ticker","symbol":"ETH/USD","data":"test"}`)
			hub.Messages <- msg

			err := awaitMessage(secondConn, msg, 2*time.Second)
			So(err, ShouldBeNil)
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

	uiChannel := make(chan []byte, 1000)
	price, balance, desk := testHubDeps(uiChannel)
	hub, err := NewHub(ctx, price, balance, desk, uiChannel)

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
