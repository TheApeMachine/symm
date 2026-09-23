package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func tapeRow(session string, sequence int64, payload string) []byte {
	// Strings and integer fields in this fixture always marshal successfully.
	record := CaptureRecord{ID: fmt.Sprintf("%s:%d", session, sequence), Session: session, Sequence: sequence,
		ReceivedAt: "2026-09-22T12:00:00.123456Z", ReceivedTime: "2026-09-22T12:00:00.123456789Z",
		Endpoint: "wss://fixture", Payload: []byte(payload)}
	row, err := json.Marshal(record)

	if err != nil {
		panic(err)
	}
	return row
}

func TestTapeWrite(t *testing.T) {
	Convey("Given shuffled archive rows from independent capture sessions", t, func() {
		client := Tape_ServerToClient(NewTape())
		defer client.Release()
		ctx := context.Background()

		for _, row := range [][]byte{tapeRow("second", 0, "other"), tapeRow("first", 10, "ten"), tapeRow("first", 2, "two"), tapeRow("first", 2, "two")} {
			So(client.Write(ctx, func(params Tape_write_Params) error { return params.SetRow(row) }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, TapeResult_Which_idle)
			release()
		}

		Convey("Snapshot exhaustion orders numeric cursors, deduplicates, and retains exact metadata", func() {
			So(client.Write(ctx, func(params Tape_write_Params) error { params.SetExhausted(true); return nil }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			for index, expected := range []string{"two", "ten", "other"} {
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, TapeResult_Which_frame)
				payload, err := result.Frame().Payload()
				So(err, ShouldBeNil)
				So(string(payload), ShouldEqual, expected)
				So(result.Frame().Sequence(), ShouldEqual, []int64{2, 10, 0}[index])
				// The frames still to replay keep an otherwise quiet graph running.
				So(result.Pending(), ShouldEqual, uint64(2-index))
				received, err := result.Frame().ReceivedAt()
				So(err, ShouldBeNil)
				So(received, ShouldEqual, "2026-09-22T12:00:00.123456789Z")
				row, err := result.Frame().Row()
				So(err, ShouldBeNil)
				var record CaptureRecord
				So(json.Unmarshal(row, &record), ShouldBeNil)
				So(record.Sequence, ShouldEqual, result.Frame().Sequence())
				So(record.ReceivedTime, ShouldEqual, received)
				So(string(record.Payload), ShouldEqual, expected)
				release()
			}
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, TapeResult_Which_exhausted)
			So(result.Pending(), ShouldEqual, 0)
		})
	})
}

func TestTapeAppend(t *testing.T) {
	Convey("Given capture identity validation", t, func() {
		tape := NewTape()
		So(tape.append(tapeRow("capture", 2, "original")), ShouldBeNil)
		Convey("Conflicting duplicates fail without replacing the original", func() {
			So(tape.append(tapeRow("capture", 2, "different")), ShouldNotBeNil)
			So(string(tape.records["capture:2"].Payload), ShouldEqual, "original")
		})
		Convey("Legacy and malformed rows cannot invent a sequence", func() {
			for _, row := range []string{`{}`, `{"capture_sequence":null}`, `not JSON`} {
				So(tape.append([]byte(row)), ShouldNotBeNil)
			}
		})
	})
}

func BenchmarkTapeAppend(b *testing.B) {
	rows := make([][]byte, 1024)

	for index := range rows {
		rows[index] = tapeRow("capture", int64(index), `{"channel":"ticker","data":[{"symbol":"BTC/USD","last":123.45}]}`)
	}
	b.ReportAllocs()

	for b.Loop() {
		tape := NewTape()

		for _, row := range rows {
			if err := tape.append(row); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkTapeWrite(b *testing.B) {
	rows := make([][]byte, 1024)

	for index := range rows {
		rows[index] = tapeRow("capture", int64(index), `{"channel":"ticker","data":[{"symbol":"BTC/USD","last":123.45}]}`)
	}
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		client := Tape_ServerToClient(NewTape())

		for index := len(rows) - 1; index >= 0; index-- {
			err := client.Write(ctx, func(params Tape_write_Params) error { return params.SetRow(rows[index]) })

			if err != nil {
				b.Fatal(err)
			}
		}

		if err := client.Write(ctx, func(params Tape_write_Params) error { params.SetExhausted(true); return nil }); err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}

		for range rows {
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()

			if err != nil {
				b.Fatal(err)
			}

			if result.Which() != TapeResult_Which_frame {
				b.Fatal("missing ordered frame")
			}
			release()
		}
		client.Release()
	}
}
