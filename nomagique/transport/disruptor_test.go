package transport_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

func publish(
	ctx context.Context, client transport.Disruptor, payload []byte, admit bool,
) error {
	err := client.Write(ctx, func(params transport.Disruptor_write_Params) error {
		params.SetCapacity(1024)
		params.SetWriters(1)
		params.SetAdmit(admit)

		if len(payload) == 0 {
			return nil
		}

		return params.SetData(payload)
	})

	if err != nil {
		return err
	}

	return client.WaitStreaming()
}

func collect(
	ctx context.Context, client transport.Disruptor,
) (stages [4][]byte, status runtime.Status, err error) {
	future, release := client.Done(ctx, nil)
	defer release()

	results, err := future.Struct()

	if err != nil {
		return stages, status, err
	}

	readers := [4]func() ([]byte, error){
		results.Stage1, results.Stage2, results.Stage3, results.Stage4,
	}

	for index, read := range readers {
		observed, err := read()

		if err != nil {
			return stages, status, err
		}

		stages[index] = append([]byte(nil), observed...)
	}

	return stages, results.Status(), nil
}

func TestDisruptor(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Disruptor node", t, func() {
		client := transport.Disruptor_ServerToClient(transport.NewDisruptor(ctx))

		Convey("It publishes nothing until it is admitted", func() {
			So(publish(ctx, client, []byte("early"), false), ShouldBeNil)

			stages, status, err := collect(ctx, client)
			So(err, ShouldBeNil)
			So(status, ShouldEqual, runtime.Status(runtime.WAITING))
			So(len(stages[0]), ShouldEqual, 0)
		})

		Convey("Every stage observes an admitted observation", func() {
			So(publish(ctx, client, nil, true), ShouldBeNil)
			So(publish(ctx, client, []byte("observation"), true), ShouldBeNil)

			deadline := time.After(5 * time.Second)
			var stages [4][]byte

			for {
				collected, _, err := collect(ctx, client)
				So(err, ShouldBeNil)

				for index, observed := range collected {
					if len(observed) > 0 {
						stages[index] = observed
					}
				}

				settled := true

				for _, observed := range stages {
					if len(observed) == 0 {
						settled = false
					}
				}

				if settled {
					break
				}

				select {
				case <-deadline:
					t.Fatalf("stages did not observe the publication: %v", stages)
				default:
					time.Sleep(time.Millisecond)
				}
			}

			for _, observed := range stages {
				So(string(observed), ShouldEqual, "observation")
			}
		})

		Convey("It reports its ring pressure", func() {
			So(publish(ctx, client, nil, true), ShouldBeNil)

			for index := 0; index < 8; index++ {
				So(publish(ctx, client, []byte("observation"), true), ShouldBeNil)
			}

			deadline := time.After(5 * time.Second)

			for {
				future, release := client.Done(ctx, nil)
				results, err := future.Struct()
				So(err, ShouldBeNil)

				backlog := results.Backlog()
				release()

				if backlog > 0 {
					break
				}

				select {
				case <-deadline:
					t.Fatal("ring reported no pressure")
				default:
					time.Sleep(time.Millisecond)
				}
			}
		})
	})
}
