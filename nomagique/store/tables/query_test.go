package tables

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	. "github.com/smartystreets/goconvey/convey"
)

/* queryFixture configures the real node and observes its streaming-write barrier. */
func queryFixture(t testing.TB, setup []string) Query {
	t.Helper()
	client := Query_ServerToClient(NewQuery())
	t.Cleanup(client.Release)
	err := client.Write(context.Background(), func(params Query_write_Params) error {
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
	})
	if err != nil {
		t.Fatal(err)
	}
	future, release := client.Done(context.Background(), nil)
	defer release()
	if _, err := future.Struct(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestQueryQuery(t *testing.T) {
	Convey("The Cap'n Proto query node owns a real DuckDB session", t, func() {
		client := queryFixture(t, []string{"LOAD arrow", "CREATE TABLE observations(epoch BIGINT, symbol VARCHAR, value DECIMAL(18,4))", "INSERT INTO observations VALUES (1, 'BTC/USD', 12.3456), (2, 'ETH/USD', 99.5)"})
		Convey("A bound epoch selects only its records and preserves decimal values", func() {
			future, release := client.Query(context.Background(), func(params Query_query_Params) error {
				params.SetJson(true)
				if err := params.SetSql("SELECT * FROM observations WHERE epoch = CAST(? AS BIGINT)"); err != nil {
					return err
				}
				parameters, err := params.NewParameters(1)
				if err != nil {
					return err
				}
				return parameters.Set(0, "1")
			})
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			output, err := result.Out()
			So(err, ShouldBeNil)
			So(string(output), ShouldEqual, `[{"epoch":1,"symbol":"BTC/USD","value":12.3456}]`)
		})
		Convey("Arrow retains rows and nested cells without exploding the result", func() {
			future, release := client.Query(context.Background(), func(params Query_query_Params) error {
				return params.SetSql("SELECT symbol, [epoch, epoch + 1] AS stamps FROM observations ORDER BY epoch")
			})
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			output, err := result.Out()
			So(err, ShouldBeNil)
			reader, err := ipc.NewReader(bytes.NewReader(output))
			So(err, ShouldBeNil)
			defer reader.Release()
			So(reader.Next(), ShouldBeTrue)
			So(reader.RecordBatch().NumRows(), ShouldEqual, 2)
			So(reader.RecordBatch().Column(1).ValueStr(0), ShouldEqual, "[1,2]")
			So(reader.Next(), ShouldBeFalse)
			So(reader.Err(), ShouldBeNil)
		})
		Convey("An invalid table fails and a later valid query still succeeds", func() {
			future, release := client.Query(context.Background(), func(params Query_query_Params) error {
				params.SetJson(true)
				return params.SetSql("SELECT * FROM absent")
			})
			_, err := future.Struct()
			release()
			So(err, ShouldNotBeNil)
			future, release = client.Query(context.Background(), func(params Query_query_Params) error {
				params.SetJson(true)
				return params.SetSql("SELECT * FROM observations WHERE epoch = 999")
			})
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			output, err := result.Out()
			So(err, ShouldBeNil)
			So(json.Valid(output), ShouldBeTrue)
			So(string(output), ShouldEqual, "[]")
		})
	})
}

func BenchmarkQueryQuery(b *testing.B) {
	client := queryFixture(b, []string{"CREATE TABLE observations AS SELECT range AS epoch, 'BTC/USD' AS symbol, range / 10.0 AS value FROM range(4096)"})
	b.ResetTimer()
	for b.Loop() {
		future, release := client.Query(context.Background(), func(params Query_query_Params) error {
			params.SetJson(true)
			return params.SetSql("SELECT * FROM observations ORDER BY epoch")
		})
		result, err := future.Struct()
		if err != nil {
			release()
			b.Fatal(err)
		}
		output, err := result.Out()
		if err != nil {
			release()
			b.Fatal(err)
		}
		if len(output) == 0 {
			release()
			b.Fatal("missing rows")
		}
		release()
	}
}

func TestQueryDone(t *testing.T) {
	Convey("Analytical work cannot block the graph's configuration and status calls", t, func() {
		server := NewQuery()
		client := Query_ServerToClient(server)
		defer client.Release()
		So(client.Write(context.Background(), nil), ShouldBeNil)
		configured, configuredRelease := client.Done(context.Background(), nil)
		_, err := configured.Struct()
		configuredRelease()
		So(err, ShouldBeNil)

		// Hold the analytical session busy while the graph completes another cycle.
		server.running.Lock()
		queued, queuedRelease := client.Query(context.Background(), func(params Query_query_Params) error {
			params.SetJson(true)
			return params.SetSql("SELECT 42 AS value")
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		So(client.Write(ctx, nil), ShouldBeNil)
		status, statusRelease := client.Done(ctx, nil)
		result, err := status.Struct()
		server.running.Unlock()
		So(err, ShouldBeNil)
		So(result.Configured(), ShouldBeTrue)
		statusRelease()
		cancel()
		_, err = queued.Struct()
		queuedRelease()
		So(err, ShouldBeNil)
	})
}

func TestQueryWrite(t *testing.T) {
	Convey("Authored SQL cannot block market stages and superseded cursors never publish rows", t, func() {
		server := NewQuery()
		client := Query_ServerToClient(server)
		defer client.Release()
		server.running.Lock()
		var unlocked bool
		defer func() {
			if !unlocked {
				server.running.Unlock()
			}
		}()
		submit := func(value string) {
			So(client.Write(context.Background(), func(args Query_write_Params) error {
				if err := args.SetSql("SELECT CAST(? AS BIGINT) AS value"); err != nil {
					return err
				}
				parameters, err := args.NewParameters(1)
				if err != nil {
					return err
				}
				return parameters.Set(0, value)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}
		submit("1")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		pending, release := client.Done(ctx, nil)
		state, err := pending.Struct()
		So(err, ShouldBeNil)
		So(state.Pending(), ShouldBeTrue)
		bytes, err := state.Out()
		So(err, ShouldBeNil)
		So(bytes, ShouldBeEmpty)
		release()
		submit("2")
		server.running.Unlock()
		unlocked = true
		var output string
		var superseded uint64
		deadline := time.Now().Add(5 * time.Second)
		for output == "" && time.Now().Before(deadline) {
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			bytes, err := result.Out()
			So(err, ShouldBeNil)
			output = string(bytes)
			superseded = result.Superseded()
			release()
			if output == "" {
				time.Sleep(time.Millisecond)
			}
		}
		So(output, ShouldEqual, `[{"value":2}]`)
		So(superseded, ShouldEqual, 1)
		// A completed result is consumed exactly once.
		future, release := client.Done(context.Background(), nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		bytes, err = result.Out()
		So(err, ShouldBeNil)
		So(bytes, ShouldBeEmpty)
	})
}

/* BenchmarkQueryWrite exercises authored SQL admission and completion over the node protocol. */
func BenchmarkQueryWrite(b *testing.B) {
	client := Query_ServerToClient(NewQuery())
	defer client.Release()
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		if err := client.Write(ctx, func(args Query_write_Params) error {
			if err := args.SetSql("SELECT CAST(? AS BIGINT) AS sequence, ? AS vocabulary"); err != nil {
				return err
			}
			parameters, err := args.NewParameters(2)

			if err != nil {
				return err
			}

			if err := parameters.Set(0, "9007199254740995"); err != nil {
				return err
			}
			return parameters.Set(1, "settled-regions")
		}); err != nil {
			b.Fatal(err)
		}

		for {
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()

			if err != nil {
				release()
				b.Fatal(err)
			}
			output, err := result.Out()
			completed := len(output) > 0

			if err != nil {
				release()
				b.Fatal(err)
			}

			if completed && string(output) != `[{"sequence":9007199254740995,"vocabulary":"settled-regions"}]` {
				b.Errorf("unexpected query result: %s", output)
			}
			release()

			if completed {
				break
			}
		}
	}
}
