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
		"LOAD arrow", "ATTACH ':memory:' AS symmtables", "CREATE SCHEMA symmtables.hindsight", "CREATE SCHEMA symmtables.symm",
		"CREATE TABLE symmtables.hindsight.runs(epoch BIGINT, started_at TIMESTAMPTZ, code_commit VARCHAR, build_id VARCHAR, config_digest VARCHAR, status VARCHAR)",
		"INSERT INTO symmtables.hindsight.runs VALUES (1, '2026-09-26 07:00:00+00', 'commit-one', 'build-one', 'config-one', 'closed'), (2, '2026-09-26 08:00:00+00', 'commit-two', 'build-two', 'config-two', 'running')",
		"CREATE TABLE symmtables.hindsight.measurements(epoch BIGINT, symbol VARCHAR)",
		"INSERT INTO symmtables.hindsight.measurements VALUES (1,'BTC/USD'),(1,'BTC/USD'),(2,'ETH/USD')",
		"CREATE TABLE symmtables.hindsight.excursions(epoch BIGINT, id VARCHAR, symbol VARCHAR, anchor_tick BIGINT, profit DECIMAL(18,4))",
		"INSERT INTO symmtables.hindsight.excursions VALUES (1,'fragment-one','BTC/USD',10,12.3456),(2,'fragment-two','ETH/USD',20,-4.50)",
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
	server := &HTTPServerServer{System: runtime.NewSystem(ctx, "inspection.test"), wsServer: websocket.NewWebSocketServer(ctx), webrtcServer: webrtc.NewWebRTCServer(ctx), incoming: lf.NewQueue[[]byte]()}
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
		Convey("Historical records are returned in the frontend's shapes, filtered by epoch", func() {
			cases := []struct{ path, contains, excludes string }{
				{"/hindsight/runs", `"startedAt":"2026-09-26`, "missing"},
				{"/hindsight/symbols?run=1", `["BTC/USD"]`, "ETH/USD"},
				{"/hindsight/excursions?epoch=1", `"profit":12.3456`, "fragment-two"},
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
			for _, statement := range []string{"CREATE TEMP VIEW selected AS SELECT * FROM symmtables.hindsight.runs WHERE epoch=2", "SELECT epoch FROM selected"} {
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
