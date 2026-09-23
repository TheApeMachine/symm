package data_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func insert(
	ctx context.Context, client data.Insert, path string, value float64, payload []byte,
) ([]byte, error) {
	err := client.Write(ctx, func(params data.Insert_write_Params) error {
		params.SetValue(value)

		if err := params.SetPath(path); err != nil {
			return err
		}

		if len(payload) == 0 {
			return nil
		}

		return params.SetData(payload)
	})

	if err != nil {
		return nil, err
	}

	if err := client.WaitStreaming(); err != nil {
		return nil, err
	}

	future, release := client.Done(ctx, nil)
	defer release()

	results, err := future.Struct()

	if err != nil {
		return nil, err
	}

	out, err := results.Out()

	if err != nil {
		return nil, err
	}

	return bytes.Clone(out), nil
}

func TestInsert(t *testing.T) {
	ctx := context.Background()

	Convey("Given an Insert primitive", t, func() {
		client := data.Insert_ServerToClient(data.NewInsert(ctx))

		Convey("It names a value into an empty structure", func() {
			out, err := insert(ctx, client, "spread", 1.5, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["spread"], ShouldEqual, 1.5)
		})

		Convey("It inserts into an existing structure without losing fields", func() {
			payload, err := sonic.Marshal(map[string]any{"bid": 100.0})
			So(err, ShouldBeNil)

			out, err := insert(ctx, client, "ask", 101.0, payload)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["bid"], ShouldEqual, 100.0)
			So(document["ask"], ShouldEqual, 101.0)
		})

		Convey("It creates the objects a nested path implies", func() {
			out, err := insert(ctx, client, "touch.bid.notional", 250.0, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			touch := document["touch"].(map[string]any)
			bid := touch["bid"].(map[string]any)
			So(bid["notional"], ShouldEqual, 250.0)
		})

		Convey("It indexes an array when a segment is an integer", func() {
			out, err := insert(ctx, client, "levels.1.price", 99.5, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			levels := document["levels"].([]any)
			So(len(levels), ShouldEqual, 2)
			So(levels[0], ShouldBeNil)
			So(levels[1].(map[string]any)["price"], ShouldEqual, 99.5)
		})

		Convey("Chained inserts accumulate into one structure", func() {
			first, err := insert(ctx, client, "bid", 100.0, nil)
			So(err, ShouldBeNil)

			second, err := insert(ctx, client, "ask", 101.0, first)
			So(err, ShouldBeNil)

			out, err := insert(ctx, client, "spread", 1.0, second)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["bid"], ShouldEqual, 100.0)
			So(document["ask"], ShouldEqual, 101.0)
			So(document["spread"], ShouldEqual, 1.0)
		})

		Convey("It reports a path addressing a non-object as invalid", func() {
			payload, err := sonic.Marshal(map[string]any{"bid": 100.0})
			So(err, ShouldBeNil)

			_, err = insert(ctx, client, "bid.notional", 1.0, payload)
			So(err, ShouldNotBeNil)
		})

		Convey("It reports an undefined path rather than inventing one", func() {
			_, err := insert(ctx, client, "", 1.0, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("It reports a payload that is not a structure", func() {
			_, err := insert(ctx, client, "spread", 1.0, []byte("not json"))
			So(err, ShouldNotBeNil)
		})

		Convey("It publishes its status", func() {
			err := client.Write(ctx, func(params data.Insert_write_Params) error {
				params.SetValue(1.0)
				return params.SetPath("spread")
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status(runtime.READY))
		})
	})
}

/* TestInsertWrite preserves graph-carried values without retaining a prior document. */
func TestInsertWrite(t *testing.T) {
	Convey("Given a node inserting structured truth into capture records", t, func() {
		ctx := context.Background()
		client := data.Insert_ServerToClient(data.NewInsert(ctx))
		defer client.Release()
		for _, fixture := range []struct{ document, value, expected string }{
			{`{"sequence":18446744073709551615}`, `{"action":"ENTER","holding":false}`, `{"sequence":18446744073709551615,"truth":{"action":"ENTER","holding":false}}`},
			{``, `{"action":"EXIT","holding":true}`, `{"truth":{"action":"EXIT","holding":true}}`},
		} {
			So(client.Write(ctx, func(args data.Insert_write_Params) error {
				if err := args.SetPath("truth"); err != nil {
					return err
				}
				if err := args.SetData([]byte(fixture.document)); err != nil {
					return err
				}
				return args.SetJson([]byte(fixture.value))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			raw, err := result.Out()
			So(err, ShouldBeNil)
			var actual, expected any
			actualDecoder := json.NewDecoder(bytes.NewReader(raw))
			actualDecoder.UseNumber()
			expectedDecoder := json.NewDecoder(bytes.NewReader([]byte(fixture.expected)))
			expectedDecoder.UseNumber()
			So(actualDecoder.Decode(&actual), ShouldBeNil)
			So(expectedDecoder.Decode(&expected), ShouldBeNil)
			So(actual, ShouldResemble, expected)
			release()
		}
	})
}

/* BenchmarkInsertWrite measures structured insertion through the node protocol. */
func BenchmarkInsertWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Insert_ServerToClient(data.NewInsert(ctx))
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args data.Insert_write_Params) error {
			if err := args.SetPath("truth"); err != nil {
				return err
			}
			if err := args.SetData([]byte(`{"sequence":18446744073709551615}`)); err != nil {
				return err
			}
			return args.SetJson([]byte(`{"action":"ENTER","holding":false}`))
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestInsertWriteUnique(t *testing.T) {
	Convey("Given a unique insertion at an explicit path", t, func() {
		for _, example := range []struct {
			payload, value    string
			inserted, invalid bool
		}{
			{`{}`, `{"action":"ENTER","holding":false}`, true, false},
			{`{"example":{"action":"ENTER","holding":false}}`, `{"holding":false,"action":"ENTER"}`, false, false},
			{`{"example":{"action":"ENTER","holding":false}}`, `{"action":"WAIT","holding":false}`, false, true},
		} {
			client := data.Insert_ServerToClient(data.NewInsert(context.Background()))
			defer client.Release()
			err := client.Write(context.Background(), func(params data.Insert_write_Params) error {
				params.SetUnique(true)

				if err := params.SetPath("example"); err != nil {
					return err
				}

				if err := params.SetData([]byte(example.payload)); err != nil {
					return err
				}

				return params.SetJson([]byte(example.value))
			})

			if err == nil {
				err = client.WaitStreaming()
			}

			if example.invalid {
				So(err, ShouldNotBeNil)
				continue
			}

			So(err, ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Inserted(), ShouldEqual, example.inserted)
			release()
		}
	})
}

func TestInsertWriteUnsigned(t *testing.T) {
	Convey("Given an unsigned graph port encoded into a JSON document", t, func() {
		ctx := context.Background()
		client := data.Insert_ServerToClient(data.NewInsert(ctx))
		defer client.Release()
		for _, expected := range []uint64{0, 9007199254740993, ^uint64(0)} {
			So(client.Write(ctx, func(args data.Insert_write_Params) error {
				if err := args.SetPath("cursor.index"); err != nil {
					return err
				}
				if err := args.SetEncoding("uint64"); err != nil {
					return err
				}
				args.SetUnsigned(expected)
				return args.SetData([]byte(`{"preserved":true}`))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			payload, err := result.Out()
			So(err, ShouldBeNil)
			var document struct {
				Preserved bool
				Cursor    struct{ Index uint64 }
			}
			So(json.Unmarshal(payload, &document), ShouldBeNil)
			So(document.Preserved, ShouldBeTrue)
			So(document.Cursor.Index == expected, ShouldBeTrue)
			release()
		}
	})
}
