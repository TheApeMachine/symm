package broker

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

type spySink struct {
	keys   []string
	bodies [][]byte
}

func (sink *spySink) WriteAtPrefix(key string, value []byte) (int, error) {
	sink.keys = append(sink.keys, key)
	sink.bodies = append(sink.bodies, append([]byte(nil), value...))
	return len(value), nil
}

func TestLevel3Tape(t *testing.T) {
	Convey("Level3Tape batches verbatim frames into NDJSON parts", t, func() {
		previous := system.Cfg.Market.Level3
		system.Cfg.Market.Level3 = &system.Level3{
			Record:           true,
			RecordFlushBytes: 1 << 20,
			RecordFlushLines: 2,
		}
		t.Cleanup(func() { system.Cfg.Market.Level3 = previous })

		sink := &spySink{}
		tape := NewLevel3Tape(sink, "run-a")

		tape.Record([]byte(`{"channel":"level3","type":"update","data":[]}`))
		So(sink.keys, ShouldBeEmpty)

		tape.Record([]byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD"}]}`))
		So(len(sink.keys), ShouldEqual, 1)
		So(sink.keys[0], ShouldEqual, "market/level3/run-a/0/0000000000000000000.ndjson")

		body := string(sink.bodies[0])
		So(strings.Count(body, "\n"), ShouldEqual, 2)
		So(body, ShouldContainSubstring, `"type":"update"`)
		So(body, ShouldContainSubstring, `"type":"snapshot"`)
		So(body, ShouldNotContainSubstring, "\n\n")

		tape.Record([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"ETH/USD"}]}`))
		So(len(sink.keys), ShouldEqual, 1)

		tape.Flush()
		So(len(sink.keys), ShouldEqual, 2)
		So(sink.keys[1], ShouldEqual, "market/level3/run-a/0/0000000000000000001.ndjson")
	})

	Convey("Level3Tape skips recording when disabled", t, func() {
		previous := system.Cfg.Market.Level3
		system.Cfg.Market.Level3 = &system.Level3{
			Record:           false,
			RecordFlushBytes: 64,
			RecordFlushLines: 1,
		}
		t.Cleanup(func() { system.Cfg.Market.Level3 = previous })

		sink := &spySink{}
		tape := NewLevel3Tape(sink, "run-b")
		tape.Record([]byte(`{"channel":"level3"}`))
		tape.Flush()
		So(sink.keys, ShouldBeEmpty)
	})
}
