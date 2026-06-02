package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestWriterWrite(t *testing.T) {
	convey.Convey("Given an audit file path", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")

		viper.Set("trading.audit.file", path)
		defer viper.Set("trading.audit.file", "")

		writer, err := OpenWriter()

		convey.So(err, convey.ShouldBeNil)
		convey.So(writer, convey.ShouldNotBeNil)

		writeErr := writer.Write(map[string]any{
			"audit_event": "playbook_walk",
			"symbol":      "BTC/EUR",
			"verdict":     "limit",
		})

		convey.So(writeErr, convey.ShouldBeNil)
		convey.So(writer.Close(), convey.ShouldBeNil)

		raw, readErr := os.ReadFile(path)

		convey.So(readErr, convey.ShouldBeNil)
		convey.So(string(raw), convey.ShouldContainSubstring, `"audit_event":"playbook_walk"`)
		convey.So(string(raw), convey.ShouldContainSubstring, `"event":"audit"`)
	})
}

func TestWriterRotate(t *testing.T) {
	convey.Convey("Given a tiny rotation threshold", t, func() {
		path := filepath.Join(t.TempDir(), "audit-rotate.jsonl")

		viper.Set("trading.audit.file", path)
		viper.Set("trading.audit.max_mb", 0)
		defer viper.Set("trading.audit.file", "")
		defer viper.Set("trading.audit.max_mb", 0)

		writer, err := OpenWriter()

		convey.So(err, convey.ShouldBeNil)
		writer.maxBytes = 128

		for range 8 {
			convey.So(writer.Write(map[string]any{
				"audit_event": "playbook_walk",
				"payload":     strings.Repeat("x", 32),
			}), convey.ShouldBeNil)
		}

		convey.So(writer.Close(), convey.ShouldBeNil)

		_, statErr := os.Stat(path + ".1")

		convey.So(statErr, convey.ShouldBeNil)
	})
}

func TestWriterMissingPath(t *testing.T) {
	convey.Convey("Given no audit path", t, func() {
		viper.Set("trading.audit.file", "")
		defer viper.Set("trading.audit.file", "")

		writer, err := OpenWriter()

		convey.Convey("It should not install a writer", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(writer, convey.ShouldBeNil)
		})
	})
}

func BenchmarkWriterWrite(b *testing.B) {
	path := filepath.Join(b.TempDir(), "audit-bench.jsonl")

	viper.Set("trading.audit.file", path)
	defer viper.Set("trading.audit.file", "")

	writer, err := OpenWriter()

	if err != nil || writer == nil {
		b.Fatal(err)
	}

	defer writer.Close()

	frame := map[string]any{
		"audit_event": "playbook_walk",
		"symbol":      "BTC/EUR",
		"steps":       []map[string]any{{"depth": 0, "pass": true}},
	}

	for b.Loop() {
		if err := writer.Write(frame); err != nil {
			b.Fatal(err)
		}
	}
}
