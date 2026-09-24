package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func tapeRow(session string, sequence int64, payload string, received ...string) []byte {
	receivedTime := "2026-09-22T12:00:00.123456789Z"

	if len(received) > 0 {
		receivedTime = received[0]
	}

	// Strings and integer fields in this fixture always marshal successfully.
	record := CaptureRecord{ID: fmt.Sprintf("%s:%d", session, sequence), Session: session, Sequence: sequence,
		ReceivedAt: "2026-09-22T12:00:00.123456Z", ReceivedTime: receivedTime,
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

		rows := [][]byte{
			tapeRow("second", 0, `"other"`),
			tapeRow("first", 10, `"ten"`),
			tapeRow("first", 2, `"two"`),
			tapeRow("first", 2, `"two"`),
			tapeRow("first", 11, `"eleven"`, "2026-09-22T12:00:01.5Z"),
			tapeRow("first", 12, `"lagging"`, "2026-09-22T12:00:00.9Z"),
		}

		for _, row := range rows {
			So(client.Write(ctx, func(params Tape_write_Params) error {
				if err := params.SetEnvelope("market"); err != nil {
					return err
				}
				list, err := params.NewRows(1)
				if err != nil {
					return err
				}
				return list.Set(0, row)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, TapeResult_Which_idle)
			release()
		}

		Convey("Snapshot exhaustion replays each session a captured second at a time, ordered and deduplicated", func() {
			So(client.Write(ctx, func(params Tape_write_Params) error {
				params.SetExhausted(true)
				return params.SetEnvelope("market")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			for index, expected := range []struct {
				session   string
				payloads  []string
				sequences []int64
			}{
				{"first", []string{`"two"`, `"ten"`}, []int64{2, 10}},
				{"first", []string{`"eleven"`, `"lagging"`}, []int64{11, 12}},
				{"second", []string{`"other"`}, []int64{0}},
			} {
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, TapeResult_Which_frames)
				frames := result.Frames()
				session, err := frames.Session()
				So(err, ShouldBeNil)
				So(session, ShouldEqual, expected.session)
				payloads, err := frames.Payload()
				So(err, ShouldBeNil)
				sequences, err := frames.Sequence()
				So(err, ShouldBeNil)
				documents, err := frames.Documents()
				So(err, ShouldBeNil)
				So(payloads.Len(), ShouldEqual, len(expected.payloads))

				for slot, payload := range expected.payloads {
					raw, err := payloads.At(slot)
					So(err, ShouldBeNil)
					So(string(raw), ShouldEqual, payload)
					So(sequences.At(slot), ShouldEqual, expected.sequences[slot])

					document, err := documents.At(slot)
					So(err, ShouldBeNil)
					var decoded struct {
						Capture struct{ Session, Endpoint, ReceivedAt string }
						Cursor  struct{ Sequence int64 }
						Market  string
					}
					So(json.Unmarshal(document, &decoded), ShouldBeNil)
					So(decoded.Capture.Session, ShouldEqual, expected.session)
					So(decoded.Capture.Endpoint, ShouldEqual, "wss://fixture")
					So(decoded.Cursor.Sequence, ShouldEqual, expected.sequences[slot])
					So(`"`+decoded.Market+`"`, ShouldEqual, payload)
				}

				// The frames still to replay keep an otherwise quiet graph running.
				So(result.Pending(), ShouldEqual, uint64([]int{3, 1, 0}[index]))
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
			err := client.Write(ctx, func(params Tape_write_Params) error {
				if err := params.SetEnvelope("market"); err != nil {
					return err
				}
				list, err := params.NewRows(1)
				if err != nil {
					return err
				}
				return list.Set(0, rows[index])
			})

			if err != nil {
				b.Fatal(err)
			}
		}

		if err := client.Write(ctx, func(params Tape_write_Params) error {
			params.SetExhausted(true)
			return params.SetEnvelope("market")
		}); err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}

		future, release := client.Done(ctx, nil)
		result, err := future.Struct()

		if err != nil {
			b.Fatal(err)
		}

		if result.Which() != TapeResult_Which_frames {
			b.Fatal("missing ordered frames")
		}
		release()
		client.Release()
	}
}
