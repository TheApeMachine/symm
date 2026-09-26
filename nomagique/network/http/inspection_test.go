package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/network/webrtc"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"golang.design/x/lockfree/lf"
)

/* inspectionFixture uses the production route declarations against actual local tables. */
func inspectionFixture(t testing.TB, live bool) *HTTPServerServer {
	t.Helper()
	raw, err := os.ReadFile("manifest/system.json")
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Nodes map[string]struct {
			InputData map[string]struct {
				Value json.RawMessage `json:"value"`
			} `json:"inputData"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &graph); err != nil {
		t.Fatal(err)
	}
	var routes string
	if err := json.Unmarshal(graph.Nodes["server"].InputData["routes"].Value, &routes); err != nil {
		t.Fatal(err)
	}
	setup := []string{
		"LOAD arrow", "ATTACH ':memory:' AS symmtables", "CREATE SCHEMA symmtables.symm",
		"CREATE TABLE symmtables.symm.metric_cuts_v2(epoch BIGINT, sequence BIGINT, symbol VARCHAR, complete BOOLEAN, provenance VARCHAR)",
		`INSERT INTO symmtables.symm.metric_cuts_v2 VALUES (1,10,'BTC/USD',true,'{"receivedAt":"2026-09-26T07:00:00Z"}'),(1,20,'BTC/USD',false,'{"receivedAt":"2026-09-26T07:00:01Z"}'),(2,30,'ETH/USD',true,'{"receivedAt":"2026-09-26T08:00:00Z"}')`,
		"CREATE TABLE symmtables.symm.excursion_fragments_v1(epoch BIGINT, symbol VARCHAR, anchor_sequence BIGINT, ignition_sequence BIGINT, extremum_sequence BIGINT, confirmation_sequence BIGINT, anchor DOUBLE, ignition DOUBLE, extremum DOUBLE, excursion DOUBLE, has_precursor BOOLEAN)",
		"INSERT INTO symmtables.symm.excursion_fragments_v1 VALUES (1,'BTC/USD',10,11,19,20,95,100,112.3456,0.123456,true),(2,'ETH/USD',30,31,39,40,110,100,95.5,-0.045,true)",
		"CREATE TABLE symmtables.symm.paper_round_trips_v1(symbol VARCHAR, opened TIMESTAMP, closed TIMESTAMP, basis VARCHAR, proceeds VARCHAR, pnl VARCHAR)",
		"INSERT INTO symmtables.symm.paper_round_trips_v1 VALUES ('BTC/USD','2026-09-26 07:00:00','2026-09-26 08:00:00','100.00','112.3456','12.3456')",
	}
	if live {
		if err := json.Unmarshal(graph.Nodes["inspection"].InputData["setup"].Value, &setup); err != nil {
			t.Fatal(err)
		}
	}
	query := tables.Query_ServerToClient(tables.NewQuery())
	t.Cleanup(query.Release)
	if err := query.Write(context.Background(), func(params tables.Query_write_Params) error {
		statements, err := params.NewSetup(int32(len(setup)))
		if err != nil {
			return err
		}
		for index, statement := range setup {
			if err := statements.Set(index, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	future, release := query.Done(context.Background(), nil)
	if _, err := future.Struct(); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	ctx := context.Background()
	server := &HTTPServerServer{System: runtime.NewSystem(ctx, "inspection.test"), WebSocketServerServer: websocket.NewWebSocketServer(ctx), webrtcServer: webrtc.NewWebRTCServer(ctx), incoming: lf.NewQueue[[]byte]()}
	server.Transition(runtime.READY)
	client := HTTPServer_ServerToClient(server)
	t.Cleanup(client.Release)
	if err := client.Write(ctx, func(params HTTPServer_write_Params) error {
		if err := params.SetQuery(query); err != nil {
			return err
		}
		return params.SetRoutes(routes)
	}); err != nil {
		t.Fatal(err)
	}
	done, doneRelease := client.Done(ctx, nil)
	defer doneRelease()
	if _, err := done.Struct(); err != nil {
		t.Fatal(err)
	}
	return server
}

func TestInspectionServeHTTP(t *testing.T) {
	t.Chdir("../../..")
	Convey("HTTP routes call the graph-connected Cap'n Proto query node", t, func() {
		server := inspectionFixture(t, false)
		handler := server.Handler()
		Convey("The metric map describes the current native cut vocabulary", func() {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/hindsight/metric-map", nil))
			So(response.Code, ShouldEqual, 200)
			var mapping struct {
				Metrics map[string]struct {
					Source, Metric, Destinations string
				}
				Signals map[string]json.RawMessage
			}
			So(json.Unmarshal(response.Body.Bytes(), &mapping), ShouldBeNil)
			So(len(mapping.Signals), ShouldEqual, 15)
			So(mapping.Metrics["correlation_ticker:hy.correlation"].Destinations, ShouldEqual, "gather.values")
			So(mapping.Metrics["sentiment_ticker:return.out"].Destinations, ShouldEqual, "gather.values_299")
			So(mapping.Metrics["sentiment_ticker:return.out"].Source, ShouldEqual, "sentiment_ticker")
		})
		Convey("Historical records are returned in the frontend's shapes, filtered by epoch", func() {
			cases := []struct{ path, contains, excludes string }{
				{"/hindsight/runs", `"startedAt":"2026-09-26`, "missing"},
				{"/hindsight/symbols?run=1", `["BTC/USD"]`, "ETH/USD"},
				{"/hindsight/excursions?epoch=1", `"excursion":0.123456`, "ETH/USD"},
				{"/trades", `"execution":"paper"`, "missing"},
				{"/hindsight/metric-map", `"graphDigest":`, "missing"},
			}
			for _, test := range cases {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
				So(response.Code, ShouldEqual, 200)
				So(response.Body.String(), ShouldContainSubstring, test.contains)
				So(response.Body.String(), ShouldNotContainSubstring, test.excludes)
				So(json.Valid(response.Body.Bytes()), ShouldBeTrue)
			}
		})
		Convey("Workbench SQL returns decodable Arrow and keeps session views", func() {
			for _, statement := range []string{"CREATE TEMP VIEW selected AS SELECT * FROM symmtables.symm.metric_cuts_v2 WHERE epoch=2", "SELECT epoch FROM selected"} {
				requestBody, err := json.Marshal(map[string]string{"sql": statement})
				So(err, ShouldBeNil)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("POST", "/workbench/query", bytes.NewReader(requestBody)))
				So(response.Code, ShouldEqual, 200)
				So(response.Header().Get("Content-Type"), ShouldEqual, "application/vnd.apache.arrow.stream")
				if strings.HasPrefix(statement, "CREATE") {
					So(response.Body.Len(), ShouldEqual, 0)
					continue
				}
				reader, err := ipc.NewReader(response.Body)
				So(err, ShouldBeNil)
				So(reader.Next(), ShouldBeTrue)
				So(reader.RecordBatch().NumRows(), ShouldEqual, 1)
				So(reader.RecordBatch().Column(0).ValueStr(0), ShouldEqual, "2")
				So(reader.Err(), ShouldBeNil)
				reader.Release()
			}
		})
		Convey("Malformed filters cannot select another run or alter SQL", func() {
			for _, path := range []string{"/hindsight/symbols", "/hindsight/excursions?run=1%20OR%201=1"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
				So(response.Code, ShouldEqual, 400)
			}
		})
	})
}

func TestInspectionServeHTTPWarehouse(t *testing.T) {
	if os.Getenv("SYMM_TEST_WAREHOUSE") != "1" {
		t.Skip("set SYMM_TEST_WAREHOUSE=1 for read-only warehouse verification")
	}
	t.Chdir("../../..")
	Convey("The production query node reads the existing Iceberg warehouse", t, func() {
		server := inspectionFixture(t, true)
		handler := server.Handler()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/hindsight/runs", nil))
		So(response.Code, ShouldEqual, 200)
		if response.Code != 200 {
			t.Log(response.Body.String())
			return
		}
		var runs []struct {
			ID string `json:"id"`
		}
		So(json.Unmarshal(response.Body.Bytes(), &runs), ShouldBeNil)
		So(len(runs), ShouldBeGreaterThan, 0)
		if len(runs) == 0 {
			return
		}
		for _, path := range []string{"/hindsight/symbols?run=" + runs[0].ID, "/hindsight/excursions?run=" + runs[0].ID, "/trades", "/hindsight/metric-map"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 200 {
				t.Log(response.Body.String())
			}
			So(response.Code, ShouldEqual, 200)
			So(json.Valid(response.Body.Bytes()), ShouldBeTrue)
			t.Logf("%s: HTTP %d, %d bytes", path, response.Code, response.Body.Len())
		}
	})
}

func BenchmarkInspectionServeHTTP(b *testing.B) {
	b.Chdir("../../..")
	server := inspectionFixture(b, false)
	handler := server.Handler()
	b.ReportAllocs()
	b.ResetTimer()
	for _, route := range []string{"/hindsight/excursions?run=1", "/hindsight/metric-map"} {
		b.Run(route, func(b *testing.B) {
			for range b.N {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", route, nil))

				if response.Code != 200 {
					b.Fatal(response.Body.String())
				}
			}
		})
	}
}
