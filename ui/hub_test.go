package ui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
fakeReplayer reports each Stream it was asked for, writes one frame, and
then holds the stream open until its ctx ends.
*/
type fakeReplayer struct {
	started chan string
	ended   chan error
}

func (replayer *fakeReplayer) Stream(ctx context.Context, w io.Writer, run uint64, symbol string) error {
	replayer.started <- symbol

	if _, err := w.Write([]byte(symbol)); err != nil {
		replayer.ended <- err
		return err
	}

	<-ctx.Done()
	replayer.ended <- ctx.Err()
	return ctx.Err()
}

func newClient() *client {
	return &client{frames: make(chan []byte, clientQueue), done: make(chan struct{})}
}

func TestHubReplay(t *testing.T) {
	Convey("Given a hub with a replayer and one client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil)
		replayer := &fakeReplayer{started: make(chan string, 4), ended: make(chan error, 4)}
		current := newClient()

		Convey("Without a replayer the hindsight command is an error", func() {
			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000"}`)), ShouldNotBeNil)
		})

		hub.Replay(replayer)

		Convey("A run that is not a decimal epoch is an error", func() {
			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":1}`)), ShouldNotBeNil)
			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"-3"}`)), ShouldNotBeNil)
			So(hub.handleCommand(nil, []byte(`{"type":"hindsight","run":"7"}`)), ShouldNotBeNil)
		})

		Convey("The run index keeps naming symbols while a tape is open", func() {
			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000"}`)), ShouldBeNil)
			So(<-replayer.started, ShouldEqual, "")
			So(string(<-current.frames), ShouldEqual, "")

			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000","symbol":"BTC/USD"}`)), ShouldBeNil)
			So(<-replayer.started, ShouldEqual, "BTC/USD")
			So(string(<-current.frames), ShouldEqual, "BTC/USD")

			select {
			case err := <-replayer.ended:
				t.Fatalf("opening a symbol ended the index: %v", err)
			case <-time.After(50 * time.Millisecond):
			}

			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000","symbol":"ETH/USD"}`)), ShouldBeNil)
			So(<-replayer.ended, ShouldEqual, context.Canceled)
			So(<-replayer.started, ShouldEqual, "ETH/USD")

			select {
			case err := <-replayer.ended:
				t.Fatalf("switching the tape ended the index: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
		})

		Convey("A replay streams to the client that asked; a newer request cancels it", func() {
			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000","symbol":"BTC/USD"}`)), ShouldBeNil)
			So(<-replayer.started, ShouldEqual, "BTC/USD")
			So(string(<-current.frames), ShouldEqual, "BTC/USD")

			So(hub.handleCommand(current, []byte(`{"type":"hindsight","run":"1792187257188073000","symbol":"ETH/USD"}`)), ShouldBeNil)
			So(<-replayer.ended, ShouldEqual, context.Canceled)
			So(<-replayer.started, ShouldEqual, "ETH/USD")

			Convey("and closing the client ends the replay", func() {
				close(current.done)
				So(<-replayer.ended, ShouldEqual, context.Canceled)
			})
		})

		Convey("A replay waits for room in the client queue; a broadcast drops instead", func() {
			for range clientQueue {
				current.frames <- []byte("x")
			}

			hub.clients.Store(current, current)
			_, err := hub.Write([]byte("live"))
			So(err, ShouldBeNil)

			written := make(chan error, 1)
			go func() {
				_, err := current.Write([]byte("replayed"))
				written <- err
			}()

			select {
			case <-written:
				t.Fatal("a replay write into a full queue returned without room")
			case <-time.After(50 * time.Millisecond):
			}

			<-current.frames
			So(<-written, ShouldBeNil)

			close(current.done)
			_, err = current.Write([]byte("late"))
			So(err, ShouldEqual, errClientClosed)
		})

		Convey("Write clones the frame so the caller cannot mutate what is queued", func() {
			frame := []byte("replayed")
			n, err := current.Write(frame)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(frame))
			frame[0] = 'X'
			So(string(<-current.frames), ShouldEqual, "replayed")
		})
	})
}

func TestHubDetections(t *testing.T) {
	Convey("Given a hub and one dashboard client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil)
		current := newClient()
		defer close(current.done)

		Convey("Without a store the detections command is an error", func() {
			So(hub.handleCommand(current, []byte(`{"type":"detections"}`)), ShouldNotBeNil)
		})

		objects := &fakeObjects{started: make(chan string, 4), ended: make(chan error, 4)}
		hub.objects = objects

		Convey("A malformed request is an error and starts nothing", func() {
			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"x"}`)), ShouldNotBeNil)
			So(hub.handleCommand(current, []byte(`{"type":"detections","symbol":"BTC/USD","class":"up","at":"9"}`)), ShouldNotBeNil)
			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"7","symbol":"BTC/USD","at":"9"}`)), ShouldNotBeNil)
			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"7","symbol":"BTC/USD","class":"up"}`)), ShouldNotBeNil)
			So(hub.handleCommand(nil, []byte(`{"type":"detections"}`)), ShouldNotBeNil)
			So(objects.started, ShouldBeEmpty)
		})

		Convey("Runs and excursions read detections/; a tape reads that symbol", func() {
			So(hub.handleCommand(current, []byte(`{"type":"detections"}`)), ShouldBeNil)
			So(<-objects.started, ShouldEqual, "detections/")
			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"7"}`)), ShouldBeNil)
			So(<-objects.started, ShouldEqual, "detections/")
			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"7","symbol":"BTC/USD","class":"up","at":"9"}`)), ShouldBeNil)
			So(<-objects.started, ShouldEqual, "detections/up/BTC/USD/")
			So(objects.ended, ShouldBeEmpty)

			So(hub.handleCommand(current, []byte(`{"type":"detections","run":"7","symbol":"ETH/USD","class":"down","at":"12"}`)), ShouldBeNil)
			So(<-objects.ended, ShouldEqual, context.Canceled)
			So(<-objects.started, ShouldEqual, "detections/down/ETH/USD/")
			So(objects.ended, ShouldBeEmpty)
		})
	})
}

func TestHub(t *testing.T) {
	Convey("Given a ui Hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil)

		Convey("Dashboard commands: known ones apply, malformed or unknown ones are errors", func() {
			defer types.SetFocus("BTC/USD")

			So(hub.handleCommand(nil, []byte(`{"type":"focus","symbol":"ETH/USD"}`)), ShouldBeNil)
			So(types.Focus(), ShouldEqual, "ETH/USD")

			So(hub.handleCommand(nil, []byte(`{"type":`)), ShouldNotBeNil)
			So(hub.handleCommand(nil, []byte(`{"type":"teleport"}`)), ShouldNotBeNil)
			So(types.Focus(), ShouldEqual, "ETH/USD")
		})

		Convey("Hub provides a Fiber App with registered routes", func() {
			app := hub.App()
			So(app, ShouldNotBeNil)

			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			// Non-websocket request to /ws should require upgrade
			So(resp.StatusCode, ShouldEqual, http.StatusUpgradeRequired)

			mockWorkbench := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
				writer.WriteHeader(http.StatusOK)
			}))
			defer mockWorkbench.Close()

			oldURL := system.Cfg.Workbench.URL
			system.Cfg.Workbench.URL = mockWorkbench.URL
			defer func() { system.Cfg.Workbench.URL = oldURL }()

			// When workbench is running, /workbench/query proxies the query
			body := strings.NewReader(`{"sql":"SHOW ALL TABLES"}`)
			queryReq := httptest.NewRequest(http.MethodPost, "/workbench/query", body)
			queryReq.Header.Set("Content-Type", "application/json")
			queryReq.Header.Set("Origin", "http://localhost:3000")
			queryResp, err := app.Test(queryReq)
			So(err, ShouldBeNil)
			So(queryResp.StatusCode, ShouldEqual, http.StatusOK)
			So(queryResp.Header.Get("Content-Type"), ShouldEqual, "application/vnd.apache.arrow.stream")
			So(queryResp.Header.Get("Access-Control-Allow-Origin"), ShouldEqual, "http://localhost:3000")

			// When workbench target is down, /workbench/query returns 503
			system.Cfg.Workbench.URL = "http://127.0.0.1:59999/workbench/query"
			badReq := httptest.NewRequest(http.MethodPost, "/workbench/query", strings.NewReader(`{"sql":"SELECT 1"}`))
			badReq.Header.Set("Content-Type", "application/json")
			badResp, err := app.Test(badReq)
			So(err, ShouldBeNil)
			So(badResp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})
	})
}

func TestHubRetained(t *testing.T) {
	Convey("Given a hub and a connected client", t, func() {
		hub := NewHub(context.Background(), nil)
		early := newClient()
		hub.join("early", early)

		Convey("A retained frame reaches connected clients and every later one first", func() {
			frame := []byte("tree-1")
			_, err := hub.Retained().Write(frame)
			So(err, ShouldBeNil)
			frame[0] = 'X' // the hub holds its own copy
			So(string(<-early.frames), ShouldEqual, "tree-1")

			_, err = hub.Retained().Write([]byte("tree-2"))
			So(err, ShouldBeNil)
			So(string(<-early.frames), ShouldEqual, "tree-2")

			late := newClient()
			hub.join("late", late)
			So(string(<-late.frames), ShouldEqual, "tree-2")
		})

		Convey("Without a retained frame a new client's queue starts empty", func() {
			late := newClient()
			hub.join("late", late)
			So(len(late.frames), ShouldEqual, 0)
		})

		Convey("Each kept key is replayed, the tree ahead of the others", func() {
			_, err := hub.Retain("sequitur/grammar/ETH/USD").Write([]byte("eth"))
			So(err, ShouldBeNil)
			_, err = hub.Retained().Write([]byte("tree"))
			So(err, ShouldBeNil)
			_, err = hub.Retain("cognition/match/SOL/USD").Write([]byte("sol"))
			So(err, ShouldBeNil)

			late := newClient()
			hub.join("late", late)
			So(string(<-late.frames), ShouldEqual, "tree")

			rest := map[string]bool{string(<-late.frames): true, string(<-late.frames): true}
			So(rest["eth"], ShouldBeTrue)
			So(rest["sol"], ShouldBeTrue)
		})
	})
}

type fakeObjects struct {
	frames  [][]byte
	started chan string
	ended   chan error
}

func (fake *fakeObjects) Stream(ctx context.Context, prefix string, dst io.Writer) error {
	fake.started <- prefix

	for _, frame := range fake.frames {
		if _, err := dst.Write(frame); err != nil {
			fake.ended <- err
			return err
		}
	}

	<-ctx.Done()
	fake.ended <- ctx.Err()
	return ctx.Err()
}

func TestHubS3Stream(t *testing.T) {
	Convey("Given a hub and one dashboard client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil)
		current := newClient()

		Convey("A prefix request without a store is an error", func() {
			So(hub.handleCommand(current, []byte(`{"type":"s3","prefix":"position/open/"}`)), ShouldNotBeNil)
			So(hub.handleCommand(current, []byte(`{"type":"s3"}`)), ShouldNotBeNil)
			So(hub.handleCommand(nil, []byte(`{"type":"s3","prefix":"position/open/"}`)), ShouldNotBeNil)
		})

		objects := &fakeObjects{
			frames:  [][]byte{[]byte("lot-a"), []byte("lot-b")},
			started: make(chan string, 2),
			ended:   make(chan error, 2),
		}
		hub.objects = objects

		Convey("The prefix streams each object to the client that asked", func() {
			So(hub.handleCommand(current, []byte(`{"type":"s3","prefix":"position/open/"}`)), ShouldBeNil)
			So(<-objects.started, ShouldEqual, "position/open/")
			So(string(<-current.frames), ShouldEqual, "lot-a")
			So(string(<-current.frames), ShouldEqual, "lot-b")

			So(hub.handleCommand(current, []byte(`{"type":"s3","prefix":"position/closed/"}`)), ShouldBeNil)
			So(<-objects.ended, ShouldEqual, context.Canceled)
			So(<-objects.started, ShouldEqual, "position/closed/")
		})
	})
}
