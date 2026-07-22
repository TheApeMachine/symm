package system_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type mockReporter struct {
	mutex             sync.Mutex
	status            types.Status
	initializeErr     error
	initializeCalls   int
	readyOnInitialize bool
}

func newMockReporter(status types.Status) *mockReporter {
	return &mockReporter{status: status}
}

func (reporter *mockReporter) Status() types.Status {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	return reporter.status
}

func (reporter *mockReporter) SetStatus(status types.Status) {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	reporter.status = status
}

func (reporter *mockReporter) Initialize() error {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()
	reporter.initializeCalls++

	if reporter.initializeErr != nil {
		reporter.status = types.ERROR
		return reporter.initializeErr
	}

	if reporter.readyOnInitialize {
		reporter.status = types.READY
	}

	return nil
}

func (reporter *mockReporter) SetInitializeError(err error) {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	reporter.initializeErr = err
}

func (reporter *mockReporter) SetReadyOnInitialize(ready bool) {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	reporter.readyOnInitialize = ready
}

func (reporter *mockReporter) InitializeCalls() int {
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	return reporter.initializeCalls
}

func TestStageStatus(t *testing.T) {
	Convey("Given a stage with two reporters that have not reported READY yet", t, func() {
		first := newMockReporter(types.INITIALIZING)
		second := newMockReporter(types.INITIALIZING)
		stage := system.NewStage(system.StagePreflight, first, second)

		Convey("Then the stage reports INITIALIZING", func() {
			So(stage.Status(), ShouldEqual, types.INITIALIZING)
		})

		Convey("When every reporter becomes READY", func() {
			first.SetStatus(types.READY)
			second.SetStatus(types.READY)

			Convey("Then the stage reports READY", func() {
				So(stage.Status(), ShouldEqual, types.READY)
			})

			Convey("And when a reporter later drops out of READY", func() {
				stage.Status()
				second.SetStatus(types.INITIALIZING)

				Convey("Then the stage reports PENDING, not INITIALIZING", func() {
					So(stage.Status(), ShouldEqual, types.PENDING)
				})
			})
		})
	})
}

func TestStageInitialize(t *testing.T) {
	Convey("Given a stage whose reporters become ready through their own Initialize", t, func() {
		first := newMockReporter(types.INITIALIZING)
		first.SetReadyOnInitialize(true)

		second := newMockReporter(types.INITIALIZING)
		second.SetReadyOnInitialize(true)

		stage := system.NewStage(system.StagePreflight, first, second)

		Convey("When Initialize runs", func() {
			err := stage.Initialize(t.Context(), nil)

			Convey("Then it returns no error", func() {
				So(err, ShouldBeNil)
			})

			Convey("Then every reporter was initialized exactly once", func() {
				So(first.InitializeCalls(), ShouldEqual, 1)
				So(second.InitializeCalls(), ShouldEqual, 1)
			})

			Convey("Then every reporter ended up READY", func() {
				So(first.Status(), ShouldEqual, types.READY)
				So(second.Status(), ShouldEqual, types.READY)
			})
		})
	})

	Convey("Given a reporter that only becomes ready after a later event", t, func() {
		first := newMockReporter(types.PENDING)

		second := newMockReporter(types.INITIALIZING)
		second.SetReadyOnInitialize(true)

		stage := system.NewStage(system.StagePreflight, first, second)

		Convey("When Initialize runs in the background", func() {
			done := make(chan error, 1)

			go func() {
				done <- stage.Initialize(t.Context(), nil)
			}()

			Convey("Then it waits for the first reporter before touching the second", func() {
				time.Sleep(30 * time.Millisecond)
				So(second.InitializeCalls(), ShouldEqual, 0)

				first.SetStatus(types.READY)
				err := <-done

				So(err, ShouldBeNil)
				So(second.InitializeCalls(), ShouldEqual, 1)
			})
		})
	})

	Convey("Given a reporter whose Initialize fails", t, func() {
		failing := newMockReporter(types.INITIALIZING)
		failing.SetInitializeError(errors.New("boom"))

		never := newMockReporter(types.INITIALIZING)
		never.SetReadyOnInitialize(true)

		stage := system.NewStage(system.StagePreflight, failing, never)

		Convey("When Initialize runs", func() {
			err := stage.Initialize(t.Context(), nil)

			Convey("Then it returns an error and never starts the next reporter", func() {
				So(err, ShouldNotBeNil)
				So(never.InitializeCalls(), ShouldEqual, 0)
			})

			Convey("Then the stage reports ERROR", func() {
				So(stage.Status(), ShouldEqual, types.ERROR)
			})
		})
	})
}
