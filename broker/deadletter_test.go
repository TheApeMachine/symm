package broker

import (
	"context"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
)

func TestDeadLetterRecord(t *testing.T) {
	Convey("Given a dead letter recorder with audit", t, func() {
		tempDir := t.TempDir()
		viper.Set("trading.audit.file", tempDir+"/audit.jsonl")

		writerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writer, writerErr := audit.NewWriter(writerCtx)

		So(writerErr, ShouldBeNil)
		So(writer, ShouldNotBeNil)

		deadLetter := NewDeadLetter(writer)
		deadLetter.Record("order", "nil action", map[string]any{
			"symbol": "BTC/USD",
		})

		cancel()
		_ = writer.Close()

		data, readErr := os.ReadFile(tempDir + "/audit.jsonl")

		Convey("It should increment drops and write a dead letter frame", func() {
			So(readErr, ShouldBeNil)
			So(deadLetter.Drops(), ShouldEqual, 1)
			So(string(data), ShouldContainSubstring, "dead_letter")
			So(string(data), ShouldContainSubstring, "nil action")
		})
	})
}
