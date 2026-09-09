package ui

import (
	"encoding/base64"
	"encoding/json"
	fastws "github.com/fasthttp/websocket"
	fiberws "github.com/gofiber/contrib/v3/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestHubWriteFrontend proves publication is safe before any dashboard client
connects. The guard must return without touching the nil connection — it
previously checked `hub.frontend != nil` and fell through to WriteMessage on
the nil connection, panicking on the first observe tick.

The hub no longer runs on the ring, so this is no longer about protecting the
pipeline from the encode; it is about the publisher goroutine surviving a run
with nobody watching. The encode itself must not happen at all in that case,
which the allocation count proves.
*/
func TestHubWriteFrontend(t *testing.T) {
	Convey("Given a hub with no dashboard clients", t, func() {
		hub := &Hub{}
		envelope := &types.Envelope{Key: "TEST/USD"}

		Convey("Writing returns without panicking", func() {
			So(func() { hub.writeFrontend(envelope) }, ShouldNotPanic)
		})

		Convey("Writing does not allocate a discarded FlatBuffer snapshot", func() {
			allocations := testing.AllocsPerRun(100, func() {
				hub.writeFrontend(envelope)
			})

			So(allocations, ShouldEqual, 0)
		})
	})
	Convey("A failed browser write detaches the dead connection immediately", t, func() {
		accepted := make(chan *fastws.Conn, 1)
		upgrader := fastws.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Error(err)
				return
			}
			accepted <- connection
		}))
		defer server.Close()
		client, response, err := fastws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		So(err, ShouldBeNil)
		if response.Body != nil {
			defer response.Body.Close()
		}
		defer client.Close()
		connection := <-accepted
		So(connection.Close(), ShouldBeNil)
		hub := &Hub{frontend: &fiberws.Conn{Conn: connection}}
		hub.writeFrontend(&types.Envelope{Key: "TEST/USD"})
		So(hub.frontend, ShouldBeNil)
		So(testing.AllocsPerRun(10, func() { hub.writeFrontend(&types.Envelope{Key: "TEST/USD"}) }), ShouldEqual, 0)
	})

}

func TestHubSetHindsightStore(t *testing.T) {
	Convey("The Hindsight HTTP contract survives an Iceberg round trip", t, func() {
		hub := inspectionArchive(t)
		read := func(path string, target any) {
			response, err := hub.app.Test(httptest.NewRequest("GET", path, nil))
			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, 200)
			So(json.NewDecoder(response.Body).Decode(target), ShouldBeNil)
			So(response.Body.Close(), ShouldBeNil)
		}
		Convey("Run dates, identities, schema versions and position counts are readable", func() {
			var runs []map[string]any
			read("/hindsight/runs", &runs)
			So(runs[0]["id"], ShouldEqual, "run")
			So(runs[0]["startedAt"], ShouldEqual, "2026-09-09T00:00:00Z")
			So(runs[0]["positions"], ShouldEqual, 1)
			So(runs[0]["schemaVersions"], ShouldResemble, map[string]any{"state": "v1"})
		})
		Convey("Lifecycle economics retain decimal precision and absent execution stays absent", func() {
			var events []map[string]any
			read("/hindsight/lifecycle?run=run", &events)
			So(events[0], ShouldNotContainKey, "execution")
			fill := events[1]["execution"].(map[string]any)
			So(fill["avgPrice"], ShouldEqual, "123.456789012300000000")
			So(fill["feeUsdEquiv"], ShouldEqual, "0.012300000000000000")
			So(fill, ShouldNotContainKey, "cumQty")
		})
		Convey("An exact ordinal returns its own payload and complete origin", func() {
			var state map[string]any
			read("/hindsight/state?run=run&seq=2&ordinal=1", &state)
			So(state["payload"], ShouldEqual, base64.StdEncoding.EncodeToString([]byte{2}))
			envelope := state["envelope"].(map[string]any)
			So(envelope["ordinal"], ShouldEqual, 1)
			So(envelope["origin"].(map[string]any)["streamSequence"], ShouldEqual, 12)
		})
		Convey("Envelope inspection resolves parents and strips witness payloads", func() {
			var result map[string]any
			read("/hindsight/envelope?run=run&seq=2", &result)
			So(len(result["manifests"].([]any)), ShouldEqual, 1)
			witnesses := result["witnesses"].([]any)
			So(len(witnesses), ShouldEqual, 2)
			witness := witnesses[0].(map[string]any)
			So(witness, ShouldNotContainKey, "payload")
			So(witness["artifact"].(map[string]any)["kind"], ShouldEqual, "state")
			parent := witness["immediateParents"].([]any)[0].(map[string]any)
			So(parent["origin"].(map[string]any)["sequence"], ShouldEqual, 1)
		})
		Convey("Timeline reads actual captured market observations", func() {
			var timeline map[string]any
			read("/hindsight/timeline?run=run&symbol=BTC%2FUSD&symbols=1", &timeline)
			So(timeline["totalObservations"], ShouldEqual, 3)
			So(timeline["totalSymbols"], ShouldEqual, 1)
		})
		Convey("Gaps use the inspection field names", func() {
			var gaps []map[string]any
			read("/hindsight/gaps?run=run", &gaps)
			So(gaps[0]["runId"], ShouldEqual, "run")
			So(gaps[0]["sequence"], ShouldEqual, 3)
		})
		Convey("An absent ordinal reports not found", func() {
			response, err := hub.app.Test(httptest.NewRequest("GET", "/hindsight/state?run=run&seq=2&ordinal=9", nil))
			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, 404)
			So(response.Body.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkHubFindState(b *testing.B) {
	hub := inspectionArchive(b)
	b.ReportAllocs()
	for b.Loop() {
		state, found, err := hub.findState("run", 2, 1)
		if err != nil || !found || len(state.Payload) != 1 || state.Payload[0] != 2 {
			b.Fatal(state, found, err)
		}
	}
}
