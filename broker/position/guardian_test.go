package position

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPositionGuardianPublish(t *testing.T) {
	Convey("A stopped execution handler cannot block priority publication", t, func() {
		position := &Regulator{
			Holding: types.NewHolding("SHAPE/USD"),
			pending: &spot.AddOrderRequest{ClOrdId: "entry"},
		}
		entered, release := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		position.record = func(execution kraken.ExecutionData) error {
			close(entered)
			<-release
			return nil
		}
		guardian := NewGuardian(position)
		position.Guardian = guardian
		guardian.Start(t.Context())
		So(guardian.Publish(kraken.ExecutionData{
			ClientOrderID: "entry", OrderStatus: "canceled",
		}), ShouldBeNil)
		<-entered

		for range guardianCapacity - 1 {
			So(guardian.Publish(kraken.Level3Data{}), ShouldBeNil)
		}
		result := make(chan error, 1)
		go func() { result <- guardian.Publish(kraken.Level3Data{}) }()

		select {
		case err := <-result:
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "saturated")
		case <-time.After(time.Second):
			t.Fatal("full guardian ring blocked its publisher")
		}
		So(guardian.Close(), ShouldBeNil)
		So(guardian.Publish(kraken.Level3Data{}), ShouldNotBeNil)
		releaseOnce.Do(func() { close(release) })
		<-guardian.Done
		So(guardian.Watermark.Load(), ShouldEqual, guardianCapacity)
	})
}

func TestPositionGuardianStart(t *testing.T) {
	Convey("The guardian owns startup and parent cancellation", t, func() {
		guardian := NewGuardian(&Regulator{})
		So(guardian.Publish(kraken.Level3Data{}), ShouldNotBeNil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		guardian.Start(ctx)
		So(guardian.Publish(kraken.Level3Data{}), ShouldBeNil)
		cancel()
		<-guardian.Done
		So(guardian.Watermark.Load(), ShouldEqual, 1)
		So(guardian.Publish(kraken.Level3Data{}), ShouldNotBeNil)
		So(guardian.Close(), ShouldBeNil)
	})
}

func TestPositionGuardianClose(t *testing.T) {
	Convey("Closing from the consumer drains accepted events without joining itself", t, func() {
		position := &Regulator{
			Holding: types.NewHolding("SHAPE/USD"),
			pending: &spot.AddOrderRequest{ClOrdId: "entry"},
		}
		position.record = func(execution kraken.ExecutionData) error {
			if err := position.Guardian.Close(); err != nil {
				panic(err)
			}
			return nil
		}
		guardian := NewGuardian(position)
		position.Guardian = guardian
		guardian.Start(t.Context())
		So(guardian.Publish(kraken.ExecutionData{
			ClientOrderID: "entry", OrderStatus: "canceled",
		}), ShouldBeNil)

		select {
		case <-guardian.Done:
			So(guardian.Watermark.Load(), ShouldEqual, 1)
		case <-time.After(time.Second):
			t.Fatal("guardian tried to join its own handler")
		}
		So(guardian.Close(), ShouldBeNil)
	})
}

func BenchmarkPositionGuardianPublish(b *testing.B) {
	guardian := NewGuardian(&Regulator{})
	guardian.Start(b.Context())
	b.Cleanup(func() {
		if err := guardian.Close(); err != nil {
			b.Fatal(err)
		}
		<-guardian.Done
	})
	frame := kraken.Level3Data{}
	b.ReportAllocs()

	var processed uint64

	for b.Loop() {
		for range guardianCapacity {
			if err := guardian.Publish(frame); err != nil {
				b.Fatal(err)
			}
		}
		// A benchmark iteration measures a complete bounded burst, including
		// consumption, so the next burst has the same initial queue state.
		processed += uint64(guardianCapacity)
		for guardian.Watermark.Load() < processed {
			time.Sleep(time.Nanosecond)
		}
	}
}
