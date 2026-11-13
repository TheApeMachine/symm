package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestNewPinger(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewPinger is called", func() {
			pinger := NewPinger(t.Context())

			Convey("the pinger names network.pinger", func() {
				So(pinger.Name(), ShouldEqual, "network.pinger")
			})
		})
	})
}

func TestStart(t *testing.T) {
	Convey("Setup", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pinger := NewPinger(ctx)
		var writeMu sync.Mutex
		status := runtime.READY
		pinger.conn = &gorilla.Conn{}
		pinger.writeMu = &writeMu

		Convey("When Start is called", func() {
			pinger.Start(pinger.conn, &writeMu, &status)

			Convey("it returns without blocking", func() {
				So(true, ShouldBeTrue)
			})
		})
	})
}

func TestPing(t *testing.T) {
	Convey("Setup", t, func() {
		var pings atomic.Int32
		upgrader := gorilla.Upgrader{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}

			conn.SetPingHandler(func(string) error {
				pings.Add(1)
				return conn.WriteControl(gorilla.PongMessage, nil, time.Now().Add(time.Second))
			})

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
		So(err, ShouldBeNil)
		defer conn.Close()

		prev := system.Cfg.WebSocket.PingInterval
		system.Cfg.WebSocket.PingInterval = 15 * time.Millisecond
		defer func() { system.Cfg.WebSocket.PingInterval = prev }()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pinger := NewPinger(ctx)
		var writeMu sync.Mutex
		status := runtime.READY
		pinger.conn = conn
		pinger.writeMu = &writeMu

		Convey("When the pinger loop is running", func() {
			pinger.Start(conn, &writeMu, &status)

			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) && pings.Load() == 0 {
				goruntime.Gosched()
			}

			cancel()
			time.Sleep(40 * time.Millisecond)

			Convey("the server received a ping", func() {
				So(pings.Load(), ShouldBeGreaterThan, int32(0))
			})
		})
	})
}

func BenchmarkNewPinger(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewPinger(ctx)
	}
}

func BenchmarkStart(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		pinger := NewPinger(ctx)
		pinger.conn = &gorilla.Conn{}
		var writeMu sync.Mutex
		status := runtime.READY
		pinger.Start(pinger.conn, &writeMu, &status)
		cancel()
	}
}

func BenchmarkPing(b *testing.B) {
	upgrader := gorilla.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		conn.SetPingHandler(func(string) error {
			return conn.WriteControl(gorilla.PongMessage, nil, time.Now().Add(time.Second))
		})

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	var writeMu sync.Mutex

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		writeMu.Lock()
		_ = conn.WriteControl(gorilla.PingMessage, nil, time.Now().Add(5*time.Second))
		writeMu.Unlock()
	}
}
