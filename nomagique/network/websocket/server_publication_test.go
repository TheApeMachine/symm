package websocket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/ui"
)

/* publicationFixture owns the real capability and socket transport shared by tests and benchmarks. */
type publicationFixture struct {
	testing testing.TB
	context context.Context
	server  *WebSocketServerServer
	client  WebSocketServer
	venue   *httptest.Server
	joined  chan struct{}
}

func newPublicationFixture(testing testing.TB) *publicationFixture {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	testing.Cleanup(cancel)
	fixture := &publicationFixture{testing: testing, context: ctx, server: NewWebSocketServer(ctx), joined: make(chan struct{}, 1)}
	fixture.client = WebSocketServer_ServerToClient(fixture.server)
	testing.Cleanup(fixture.client.Release)
	handler := fixture.server.UpgradeHandler()
	fixture.venue = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handler(writer, request)
		fixture.joined <- struct{}{}
	}))
	testing.Cleanup(fixture.venue.Close)
	return fixture
}

func (fixture *publicationFixture) connect() *gorillaws.Conn {
	fixture.testing.Helper()
	connection, response, err := gorillaws.DefaultDialer.DialContext(fixture.context, "ws"+strings.TrimPrefix(fixture.venue.URL, "http"), nil)

	if response != nil {
		if err := response.Body.Close(); err != nil {
			fixture.testing.Fatal(err)
		}
	}

	if err != nil {
		fixture.testing.Fatal(err)
	}
	fixture.testing.Cleanup(func() {
		if err := connection.Close(); err != nil {
			fixture.testing.Error(err)
		}
	})

	select {
	case <-fixture.joined:
	case <-fixture.context.Done():
		fixture.testing.Fatal(fixture.context.Err())
	}
	return connection
}

func (fixture *publicationFixture) encode(properties map[[3]string]string) []byte {
	fixture.testing.Helper()
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		fixture.testing.Fatal(err)
	}
	defer message.Release()
	bindings, err := ui.NewRootBindings(segment)

	if err != nil {
		fixture.testing.Fatal(err)
	}
	values, err := bindings.NewValues(int32(len(properties)))

	if err != nil {
		fixture.testing.Fatal(err)
	}
	index := 0

	for key, value := range properties {
		bound := values.At(index)

		for _, err := range []error{bound.SetGraph(key[0]), bound.SetComponent(key[1]), bound.SetProp(key[2]), bound.SetValue(value)} {
			if err != nil {
				fixture.testing.Fatal(err)
			}
		}
		index++
	}
	payload, err := message.Marshal()

	if err != nil {
		fixture.testing.Fatal(err)
	}
	return payload
}

func (fixture *publicationFixture) publish(payload []byte) error {
	future, release := fixture.client.Publish(fixture.context, func(args ui.Receiver_publish_Params) error { return args.SetData(payload) })
	defer release()
	_, err := future.Struct()
	return err
}

func (fixture *publicationFixture) receive(connection *gorillaws.Conn) map[[3]string]string {
	fixture.testing.Helper()
	deadline, _ := fixture.context.Deadline()

	if err := connection.SetReadDeadline(deadline); err != nil {
		fixture.testing.Fatal(err)
	}
	kind, payload, err := connection.ReadMessage()

	if err != nil {
		fixture.testing.Fatal(err)
	}

	if kind != gorillaws.BinaryMessage {
		fixture.testing.Fatal("UI publication was not a native binary frame")
	}
	message, err := capnp.Unmarshal(payload)

	if err != nil {
		fixture.testing.Fatal(err)
	}
	defer message.Release()
	bindings, err := ui.ReadRootBindings(message)

	if err != nil {
		fixture.testing.Fatal(err)
	}
	values, err := bindings.Values()

	if err != nil {
		fixture.testing.Fatal(err)
	}
	properties := make(map[[3]string]string, values.Len())

	for index := range values.Len() {
		bound := values.At(index)
		var fields [4]string

		for index, read := range []func() (string, error){bound.Graph, bound.Component, bound.Prop, bound.Value} {
			value, err := read()

			if err != nil {
				fixture.testing.Fatal(err)
			}
			fields[index] = value
		}
		properties[[3]string{fields[0], fields[1], fields[2]}] = fields[3]
	}
	return properties
}

func TestWebSocketServerPublish(t *testing.T) {
	Convey("Native UI publications retain independent properties while a real socket is blocked", t, func() {
		fixture := newPublicationFixture(t)
		slow := fixture.connect()
		var output *socketOutput
		fixture.server.clients.Range(func(key, value any) bool {
			So(key.(*gorillaws.Conn).UnderlyingConn().(*net.TCPConn).SetWriteBuffer(1024), ShouldBeNil)
			output = value.(*socketOutput)
			return true
		})
		// Larger than the configured socket send buffer: the peer is deliberately not reading.
		large := "\"" + strings.Repeat("snapshot", 1<<19) + "\""
		blockingKey := [3]string{"ui_dashboard", "trail", "values"}
		So(fixture.publish(fixture.encode(map[[3]string]string{blockingKey: large})), ShouldBeNil)

		for {
			fixture.server.publicationMu.Lock()
			pending := len(output.bindings)
			fixture.server.publicationMu.Unlock()

			if pending == 0 {
				break
			}

			if fixture.context.Err() != nil {
				t.Fatal("socket writer never accepted its first native publication")
			}
			goruntime.Gosched()
		}
		expected := make(map[[3]string]string)

		for sequence := range 64 {
			// Same component/prop names in different authored graphs must not collide.
			for _, graph := range []string{"ui_dashboard", "ui_learning"} {
				for _, prop := range []string{"values", "status"} {
					key := [3]string{graph, "trail", prop}
					value := fmt.Sprintf("%d", sequence)
					payload := fixture.encode(map[[3]string]string{key: value})
					So(fixture.publish(payload), ShouldBeNil)
					clear(payload) // Caller storage is no longer owned by Publish.
					expected[key] = value
				}
			}
		}
		fixture.server.publicationMu.Lock()
		So(len(output.bindings), ShouldEqual, len(expected))
		So(len(fixture.server.publications), ShouldEqual, len(expected))
		fixture.server.publicationMu.Unlock()
		connected := 0
		fixture.server.clients.Range(func(_, _ any) bool { connected++; return true })
		So(connected, ShouldEqual, 1)

		Convey("The blocked peer resumes with every latest property intact", func() {
			So(fixture.receive(slow)[blockingKey], ShouldEqual, large)
			So(fixture.receive(slow), ShouldResemble, expected)
		})
		Convey("A late or reconnected peer receives an atomic current snapshot", func() {
			healthy := fixture.connect()
			So(fixture.receive(healthy), ShouldResemble, expected)
			So(healthy.WriteMessage(gorillaws.CloseMessage, gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "reconnect")), ShouldBeNil)
			reconnected := fixture.connect()
			So(fixture.receive(reconnected), ShouldResemble, expected)
		})
		Convey("A join concurrent with publication cannot block the upgrade handler", func() {
			finished := make(chan error, 1)
			final := fixture.encode(expected)

			go func() {
				for range 64 {
					if err := fixture.publish(final); err != nil {
						finished <- err
						return
					}
				}
				finished <- nil
			}()

			for range 8 {
				So(fixture.receive(fixture.connect()), ShouldResemble, expected)
			}

			select {
			case err := <-finished:
				So(err, ShouldBeNil)
			case <-fixture.context.Done():
				t.Fatal("publication stalled during socket joins")
			}
		})
		Convey("Malformed publications fail without changing the retained snapshot", func() {
			So(fixture.publish([]byte("not capnp")), ShouldNotBeNil)
			So(fixture.receive(fixture.connect()), ShouldResemble, expected)
		})
	})
}

func BenchmarkWebSocketServerPublish(b *testing.B) {
	fixture := newPublicationFixture(b)
	connection := fixture.connect()
	// One graph evaluation publishes a scalar and a 120-point authored trail for each of 15 kernels.
	properties := make(map[[3]string]string)

	for kernel := range 15 {
		component := fmt.Sprintf("kernel_%d", kernel)
		properties[[3]string{"ui_dashboard", component, "value"}] = "0.125"
		properties[[3]string{"ui_dashboard", component, "values"}] = "[" + strings.Repeat("0.125,", 119) + "0.125]"
	}
	payload := fixture.encode(properties)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()

	for b.Loop() {
		if err := fixture.publish(payload); err != nil {
			b.Fatal(err)
		}

		if len(fixture.receive(connection)) != len(properties) {
			b.Fatal("native publication lost component properties")
		}
	}
}
