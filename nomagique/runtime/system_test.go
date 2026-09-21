package runtime

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSystemFail(t *testing.T) {
	Convey("The composed runtime owner retains and exposes stage failure", t, func() {
		system := NewSystem(t.Context(), "agent")
		system.Transition(READY)
		err := errors.New("execution acceptance unknown")
		system.Error(err)
		So(errors.Is(system.Error(), err), ShouldBeTrue)
		So(system.Status(), ShouldEqual, ERROR)
		system.Error(nil)
		So(errors.Is(system.Error(), err), ShouldBeTrue)
		So(system.Status(), ShouldEqual, ERROR)
		So(system.Close(), ShouldNotBeNil)
	})
}

func TestSystemFatal(t *testing.T) {
	Convey("A fatal system retains fatal status and error querying does not downgrade", t, func() {
		system := NewSystem(t.Context(), "category")
		system.Transition(READY)
		err := errors.New("measurement invalid")
		system.Error(err)
		So(system.Status(), ShouldEqual, ERROR)
		system.Transition(FATAL)
		So(system.Status(), ShouldEqual, FATAL)

		So(errors.Is(system.Error(), err), ShouldBeTrue)
		So(system.Status(), ShouldEqual, FATAL)

		err2 := errors.New("subsequent invalid")
		system.Error(err2)
		So(system.Status(), ShouldEqual, FATAL)
	})
}

func TestSystemLogging(t *testing.T) {
	Convey("A composed runtime owner records and broadcasts Info, Warning, and Error logs", t, func() {
		system := NewSystem(t.Context(), "websocket.client")

		var hooked []LogEntry
		system.OnLog(func(entry LogEntry) {
			hooked = append(hooked, entry)
		})

		var globalHooked []LogEntry
		SetGlobalLogHook(func(sys *System, entry LogEntry) {
			if sys.Name() == "websocket.client" {
				globalHooked = append(globalHooked, entry)
			}
		})
		defer SetGlobalLogHook(nil)

		system.Info("connecting to %s", "wss://example.com")
		system.Warning("buffer is %d%% full", 80)
		system.Transition(READY)
		system.Error(errors.New("connection reset by peer"))

		logs := system.Logs()
		So(len(logs), ShouldEqual, 5)
		So(logs[0].Level, ShouldEqual, LogInfo)
		So(logs[0].Message, ShouldEqual, "connecting to wss://example.com")
		So(logs[0].System, ShouldEqual, "websocket.client")

		So(logs[1].Level, ShouldEqual, LogWarning)
		So(logs[1].Message, ShouldEqual, "buffer is 80% full")

		So(logs[2].Level, ShouldEqual, LogInfo)
		So(logs[2].Message, ShouldEqual, "websocket.client: init -> ready")

		So(logs[3].Level, ShouldEqual, LogError)
		So(logs[3].Message, ShouldEqual, "connection reset by peer")

		So(logs[4].Level, ShouldEqual, LogInfo)
		So(logs[4].Message, ShouldEqual, "websocket.client: ready -> error")

		So(len(hooked), ShouldEqual, 5)
		So(len(globalHooked), ShouldEqual, 5)

		system.ClearLogs()
		So(len(system.Logs()), ShouldEqual, 0)
	})
}

