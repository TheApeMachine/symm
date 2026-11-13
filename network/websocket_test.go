package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gorilla "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func persistentWSServer(tb testing.TB) (wsURL string, cleanup func()) {
	tb.Helper()
	upgrader := gorilla.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))

	return "ws" + strings.TrimPrefix(server.URL, "http"), server.Close
}

func TestNewWebsocketClient(t *testing.T) {
	Convey("Setup", t, func() {
		const url = "wss://ws.kraken.com/v2"

		Convey("When NewWebsocketClient is called", func() {
			client := NewWebsocketClient(t.Context(), url)

			Convey("the client starts in INIT with url and pinger", func() {
				So(client.Name(), ShouldEqual, "network.websocket")
				So(client.Status(), ShouldEqual, runtime.INIT)
				So(client.url, ShouldEqual, url)
				So(client.pinger, ShouldNotBeNil)
			})
		})
	})
}

func TestOpen(t *testing.T) {
	Convey("Setup", t, func() {
		wsURL, cleanup := persistentWSServer(t)
		defer cleanup()

		client := NewWebsocketClient(t.Context(), wsURL)

		Convey("When Open dials a live server", func() {
			So(client.Open(), ShouldBeNil)

			Convey("conn is live", func() {
				So(client.conn, ShouldNotBeNil)
				So(client.Close(), ShouldBeNil)
			})
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Setup", t, func() {
		wsURL, cleanup := persistentWSServer(t)
		defer cleanup()

		client := NewWebsocketClient(t.Context(), wsURL)
		So(client.Open(), ShouldBeNil)
		client.Transition(runtime.READY)
		defer client.Close()

		const writers = 32
		var started sync.WaitGroup
		var done sync.WaitGroup
		started.Add(writers)
		done.Add(writers)
		gate := make(chan struct{})

		Convey("When many goroutines Write concurrently", func() {
			var writeErrs []error
			var errsMu sync.Mutex

			for i := 0; i < writers; i++ {
				go func(n int) {
					defer done.Done()
					started.Done()
					<-gate

					if err := client.Write(fmt.Appendf(nil, `{"n":%d}`, n)); err != nil {
						errsMu.Lock()
						writeErrs = append(writeErrs, err)
						errsMu.Unlock()
					}
				}(i)
			}

			started.Wait()
			close(gate)
			done.Wait()

			Convey("every write succeeds", func() {
				So(len(writeErrs), ShouldEqual, 0)
			})
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Setup", t, func() {
		wsURL, cleanup := persistentWSServer(t)
		defer cleanup()

		client := NewWebsocketClient(t.Context(), wsURL)
		So(client.Open(), ShouldBeNil)

		Convey("When Close is called", func() {
			So(client.Close(), ShouldBeNil)

			Convey("system is closed", func() {
				So(client.Status(), ShouldNotEqual, runtime.READY)
			})
		})
	})
}

func BenchmarkNewWebsocketClient(b *testing.B) {
	ctx := context.Background()
	const url = "wss://ws.kraken.com/v2"

	b.ReportAllocs()

	for b.Loop() {
		_ = NewWebsocketClient(ctx, url)
	}
}

func BenchmarkOpen(b *testing.B) {
	wsURL, cleanup := persistentWSServer(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		client := NewWebsocketClient(ctx, wsURL)
		_ = client.Open()
		_ = client.Close()
	}
}

func BenchmarkWrite(b *testing.B) {
	wsURL, cleanup := persistentWSServer(b)
	defer cleanup()
	ctx := context.Background()
	client := NewWebsocketClient(ctx, wsURL)
	_ = client.Open()
	client.Transition(runtime.READY)
	defer client.Close()
	payload := []byte(`{"method":"ping"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = client.Write(payload)
	}
}

func BenchmarkClose(b *testing.B) {
	wsURL, cleanup := persistentWSServer(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		client := NewWebsocketClient(ctx, wsURL)
		_ = client.Open()
		_ = client.Close()
	}
}
