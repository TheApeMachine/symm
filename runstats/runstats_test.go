package runstats

import (
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeSink struct {
	sent      atomic.Int64
	dropped   atomic.Int64
	filtered  atomic.Int64
	throttle  atomic.Int64
	recompute atomic.Int64
	connect   atomic.Int64
	reconnect atomic.Int64
	refreshOK atomic.Int64
	refreshNO atomic.Int64
}

func (sink *fakeSink) UIFramesSent(count int64)      { sink.sent.Add(count) }
func (sink *fakeSink) UIFramesDropped(count int64)   { sink.dropped.Add(count) }
func (sink *fakeSink) UIFramesFiltered(count int64)  { sink.filtered.Add(count) }
func (sink *fakeSink) LeadlagThrottle()                { sink.throttle.Add(1) }
func (sink *fakeSink) LeadlagRecompute()               { sink.recompute.Add(1) }
func (sink *fakeSink) WSConnect()                      { sink.connect.Add(1) }
func (sink *fakeSink) WSReconnect()                    { sink.reconnect.Add(1) }
func (sink *fakeSink) TokenRefresh(success bool) {
	if success {
		sink.refreshOK.Add(1)

		return
	}

	sink.refreshNO.Add(1)
}

func TestRunstatsNoSink(t *testing.T) {
	Convey("Given no installed sink", t, func() {
		Convey("It should not panic on any counter bump", func() {
			So(func() {
				UIFramesSent(1)
				UIFramesDropped(2)
				UIFramesFiltered(3)
				LeadlagThrottle()
				LeadlagRecompute()
				WSConnect()
				WSReconnect()
				TokenRefresh(true)
				TokenRefresh(false)
			}, ShouldNotPanic)
		})
	})
}

func TestRunstatsInstall(t *testing.T) {
	Convey("Given an installed sink", t, func() {
		fake := &fakeSink{}
		Install(fake)
		t.Cleanup(func() { Install(nil) })

		UIFramesSent(4)
		UIFramesDropped(5)
		UIFramesFiltered(6)
		LeadlagThrottle()
		LeadlagRecompute()
		WSConnect()
		WSReconnect()
		TokenRefresh(true)
		TokenRefresh(false)

		Convey("It should forward every counter to the sink", func() {
			So(fake.sent.Load(), ShouldEqual, 4)
			So(fake.dropped.Load(), ShouldEqual, 5)
			So(fake.filtered.Load(), ShouldEqual, 6)
			So(fake.throttle.Load(), ShouldEqual, 1)
			So(fake.recompute.Load(), ShouldEqual, 1)
			So(fake.connect.Load(), ShouldEqual, 1)
			So(fake.reconnect.Load(), ShouldEqual, 1)
			So(fake.refreshOK.Load(), ShouldEqual, 1)
			So(fake.refreshNO.Load(), ShouldEqual, 1)
		})
	})
}

func BenchmarkUIFramesSent(b *testing.B) {
	fake := &fakeSink{}
	Install(fake)
	b.Cleanup(func() { Install(nil) })

	for b.Loop() {
		UIFramesSent(1)
	}
}
