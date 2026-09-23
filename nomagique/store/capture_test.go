package store

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type ingress struct {
	payload            []byte
	endpoint, received string
}

/* admit writes the frames that arrived in one evaluation, one slot per socket. */
func admit(client Capture, frames ...ingress) error {
	err := client.Write(context.Background(), func(params Capture_write_Params) error {
		payloads, err := params.NewPayload(int32(len(frames)))

		if err != nil {
			return err
		}
		endpoints, err := params.NewEndpoint(int32(len(frames)))

		if err != nil {
			return err
		}
		times, err := params.NewReceivedAt(int32(len(frames)))

		if err != nil {
			return err
		}

		for slot, frame := range frames {
			for _, err := range []error{
				payloads.Set(slot, frame.payload), endpoints.Set(slot, frame.endpoint), times.Set(slot, frame.received),
			} {
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	return client.WaitStreaming()
}

type capturedRow struct {
	ID         string `json:"capture_id"`
	Session    string `json:"capture_session"`
	Sequence   int64  `json:"capture_sequence"`
	Endpoint   string `json:"endpoint"`
	Payload    []byte `json:"payload"`
	ReceivedAt string `json:"received_at"`
}

/* handOut reads one evaluation's row, or nil when the capture is idle. */
func handOut(client Capture) (*capturedRow, uint64) {
	future, release := client.Done(context.Background(), nil)
	defer release()
	result, err := future.Struct()
	So(err, ShouldBeNil)

	if result.Which() == Captured_Which_idle {
		return nil, result.Pending()
	}
	data, err := result.Row().Out()
	So(err, ShouldBeNil)
	var row capturedRow
	So(json.Unmarshal(bytes.Clone(data), &row), ShouldBeNil)
	return &row, result.Pending()
}

func TestCaptureWrite(t *testing.T) {
	Convey("Given one capture session", t, func() {
		client := Capture_ServerToClient(NewCapture(context.Background()))
		defer client.Release()

		Convey("Every raw frame retains its bytes and gets a distinct identity", func() {
			identifiers := map[string]bool{}

			for _, payload := range [][]byte{[]byte(" {\"channel\":\"ticker\"}\n"), {0, 255, 7}} {
				So(admit(client, ingress{payload, "wss://test.local", "2026-09-22T12:00:00.123456789Z"}), ShouldBeNil)
				row, _ := handOut(client)
				So(row, ShouldNotBeNil)
				So(row.Payload, ShouldResemble, payload)
				So(row.ReceivedAt, ShouldEqual, "2026-09-22T12:00:00.123456789Z")
				So(identifiers[row.ID], ShouldBeFalse)
				identifiers[row.ID] = true
			}

			Convey("And an evaluation without frames is idle", func() {
				So(admit(client), ShouldBeNil)
				row, pending := handOut(client)
				So(row, ShouldBeNil)
				So(pending, ShouldEqual, 0)
			})
		})

		Convey("Frames from two sockets in one evaluation share the session in slot order", func() {
			So(admit(client,
				ingress{[]byte(`{"channel":"instrument"}`), "wss://ws.kraken.com/v2", "2026-09-22T12:00:00Z"},
				ingress{[]byte(`{"channel":"level3"}`), "wss://ws-l3.kraken.com/v2", "2026-09-22T12:00:00.5Z"},
			), ShouldBeNil)
			first, pending := handOut(client)
			So(first.Endpoint, ShouldEqual, "wss://ws.kraken.com/v2")
			So(pending, ShouldEqual, 1)

			So(admit(client), ShouldBeNil)
			second, pending := handOut(client)
			So(second.Endpoint, ShouldEqual, "wss://ws-l3.kraken.com/v2")
			So(second.Session, ShouldEqual, first.Session)
			So(second.Sequence, ShouldEqual, first.Sequence+1)
			So(pending, ShouldEqual, 0)
		})

		Convey("Missing ingress identity is rejected", func() {
			So(admit(client, ingress{[]byte("frame"), "", ""}), ShouldNotBeNil)
		})
	})
}

func BenchmarkCaptureWrite(b *testing.B) {
	client := Capture_ServerToClient(NewCapture(context.Background()))
	defer client.Release()
	ctx := context.Background()
	frame := ingress{[]byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":123.45}]}`), "wss://fixture.test", "2026-09-22T12:00:00Z"}
	b.ReportAllocs()

	for b.Loop() {
		if err := admit(client, frame); err != nil {
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
