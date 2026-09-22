package store

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureWrite(t *testing.T) {
	Convey("Given one capture session", t, func() {
		ctx := context.Background()
		client := Capture_ServerToClient(NewCapture())
		defer client.Release()
		identifiers := map[string]bool{}

		Convey("Every raw frame retains its bytes and gets a distinct identity", func() {
			for _, payload := range [][]byte{[]byte(" {\"channel\":\"ticker\"}\n"), {0, 255, 7}} {
				So(client.Write(ctx, func(params Capture_write_Params) error {
					if err := params.SetEndpoint("wss://test.local"); err != nil {
						return err
					}

					if err := params.SetReceivedAt("2026-09-22T12:00:00.123456789Z"); err != nil {
						return err
					}

					return params.SetPayload(payload)
				}), ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				data, err := result.Out()
				So(err, ShouldBeNil)
				var row struct {
					ID         string `json:"capture_id"`
					Payload    []byte `json:"payload"`
					ReceivedAt string `json:"received_at"`
				}
				So(json.Unmarshal(data, &row), ShouldBeNil)
				So(row.Payload, ShouldResemble, payload)
				So(row.ReceivedAt, ShouldEqual, "2026-09-22T12:00:00.123456789Z")
				So(identifiers[row.ID], ShouldBeFalse)
				identifiers[row.ID] = true
				release()
			}

			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			out, err := result.Out()
			So(err, ShouldBeNil)
			So(out, ShouldBeEmpty)
		})

		Convey("Missing ingress identity is rejected", func() {
			So(client.Write(ctx, func(params Capture_write_Params) error { return params.SetPayload([]byte("frame")) }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func BenchmarkCaptureWrite(b *testing.B) {
	client := Capture_ServerToClient(NewCapture())
	defer client.Release()
	ctx := context.Background()
	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":123.45}]}`)
	b.ReportAllocs()

	for b.Loop() {
		err := client.Write(ctx, func(params Capture_write_Params) error {
			if err := params.SetEndpoint("wss://fixture.test"); err != nil {
				return err
			}

			if err := params.SetReceivedAt("2026-09-22T12:00:00Z"); err != nil {
				return err
			}

			return params.SetPayload(payload)
		})

		if err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}

		future, release := client.Done(ctx, nil)
		_, err = future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}
