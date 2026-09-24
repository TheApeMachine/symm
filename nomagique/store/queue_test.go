package store

import (
	"bytes"
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type queueCall struct{ offer, retry, rewind, release string }

func step(client Queue, call queueCall) (string, uint64) {
	ctx := context.Background()
	So(client.Write(ctx, func(params Queue_write_Params) error {
		lists := []struct {
			value string
			build func() (func(int, []byte) error, error)
		}{
			{call.offer, func() (func(int, []byte) error, error) { list, err := params.NewOffer(1); return list.Set, err }},
			{call.retry, func() (func(int, []byte) error, error) { list, err := params.NewRetry(1); return list.Set, err }},
			{call.rewind, func() (func(int, []byte) error, error) { list, err := params.NewRewind(1); return list.Set, err }},
			{call.release, func() (func(int, []byte) error, error) { list, err := params.NewRelease(1); return list.Set, err }},
		}

		for _, port := range lists {
			if port.value == "" {
				continue
			}
			set, err := port.build()

			if err != nil {
				return err
			}

			if err := set(0, []byte(port.value)); err != nil {
				return err
			}
		}
		return nil
	}), ShouldBeNil)
	So(client.WaitStreaming(), ShouldBeNil)
	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()
	So(err, ShouldBeNil)

	if results.Which() == Released_Which_empty {
		return "", results.Waiting()
	}
	out, err := results.Out()
	So(err, ShouldBeNil)
	return string(bytes.Clone(out)), results.Waiting()
}

func TestQueueWrite(t *testing.T) {
	Convey("Given a queue offered three symbols", t, func() {
		client := Queue_ServerToClient(NewQueue(context.Background()))
		defer client.Release()
		out, waiting := step(client, queueCall{offer: `["BTC/USD","ETH/USD","SOL/USD"]`})
		So(out, ShouldEqual, "")
		So(waiting, ShouldEqual, 3)

		Convey("Then each release hands out the oldest, one per evaluation", func() {
			out, _ = step(client, queueCall{release: "true"})
			So(out, ShouldEqual, `"BTC/USD"`)
			out, waiting = step(client, queueCall{release: "true"})
			So(out, ShouldEqual, `"ETH/USD"`)
			So(waiting, ShouldEqual, 1)
		})

		Convey("Then offering a known value again does not queue it twice", func() {
			_, waiting = step(client, queueCall{offer: `["ETH/USD","ADA/USD"]`})
			So(waiting, ShouldEqual, 4)
		})

		Convey("Then a retried value rejoins at the back", func() {
			step(client, queueCall{release: "true"})
			out, waiting = step(client, queueCall{retry: `["BTC/USD"]`, release: "true"})
			So(out, ShouldEqual, `"ETH/USD"`)
			So(waiting, ShouldEqual, 2)
		})

		Convey("Then a rewind queues everything known again, in offer order", func() {
			step(client, queueCall{release: "true"})
			step(client, queueCall{release: "true"})
			out, waiting = step(client, queueCall{rewind: "true", release: "true"})
			So(out, ShouldEqual, `"BTC/USD"`)
			So(waiting, ShouldEqual, 2)
		})

		Convey("Then a release with nothing waiting leaves it empty", func() {
			for range 3 {
				step(client, queueCall{release: "true"})
			}
			out, waiting = step(client, queueCall{release: "true"})
			So(out, ShouldEqual, "")
			So(waiting, ShouldEqual, 0)

			Convey("And the readiness holds for the next offer, once, however many releases found it empty", func() {
				out, _ = step(client, queueCall{offer: `["QRS/USD","TUV/USD"]`})
				So(out, ShouldEqual, `"QRS/USD"`)
				out, waiting = step(client, queueCall{})
				So(out, ShouldEqual, "")
				So(waiting, ShouldEqual, 1)
			})
		})
	})

	Convey("Given an offer that is not a JSON array", t, func() {
		client := Queue_ServerToClient(NewQueue(context.Background()))
		defer client.Release()
		So(client.Write(context.Background(), func(params Queue_write_Params) error {
			list, err := params.NewOffer(1)

			if err != nil {
				return err
			}
			return list.Set(0, []byte(`"BTC/USD"`))
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
	})
}
