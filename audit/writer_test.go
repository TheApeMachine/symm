package audit

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWriterEnqueueDeduped(t *testing.T) {
	Convey("Given a writer with a short cooldown", t, func() {
		tempDir := t.TempDir()
		path := tempDir + "/audit.jsonl"
		writerCtx, cancel := context.WithCancel(context.Background())

		writer := &Writer{
			ctx:        writerCtx,
			cancel:     cancel,
			path:       path,
			queue:      make(chan auditJob, 8),
			cooldown:   500 * time.Millisecond,
			maxBytes:   1024 * 1024,
			maxBackups: 1,
			done:       make(chan struct{}),
		}

		So(writer.openFile(), ShouldBeNil)

		go writer.run()

		frame := map[string]any{"event": "playbook_eval", "symbol": "BTC/USD"}

		Convey("It should suppress duplicate gate lines inside the cooldown", func() {
			So(writer.EnqueueDeduped("btc:5/0/0", frame), ShouldBeNil)
			So(writer.EnqueueDeduped("btc:5/0/0", frame), ShouldBeNil)

			cancel()
			<-writer.done

			data, err := os.ReadFile(path)

			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, "playbook_eval")
			So(len(string(data)), ShouldBeLessThan, 300)
		})
	})
}
