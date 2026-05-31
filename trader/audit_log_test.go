package trader

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuditLogAppend(t *testing.T) {
	Convey("Given an audit log", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 1<<20, 3, time.Minute)
		So(err, ShouldBeNil)
		defer auditLog.Close()

		Convey("When lifecycle events are appended", func() {
			So(auditLog.Append("entry", map[string]any{
				"symbol": "BTC/EUR",
				"reason": "filled",
			}), ShouldBeNil)

			lines, readErr := readAuditLines(path)

			Convey("It should write one JSONL record", func() {
				So(readErr, ShouldBeNil)
				So(lines, ShouldHaveLength, 1)
				So(lines[0]["audit_event"], ShouldEqual, "entry")
			})
		})
	})
}

func TestAuditLogDedupeGateReject(t *testing.T) {
	Convey("Given an audit log with a short gate cooldown", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 1<<20, 3, time.Second)
		So(err, ShouldBeNil)
		defer auditLog.Close()

		frame := map[string]any{
			"symbol":   "VVV/EUR",
			"playbook": "trend",
			"reason":   "systemic_slump_wait",
		}

		Convey("When the same gate reject repeats inside the cooldown", func() {
			So(auditLog.Append("gate_reject", frame), ShouldBeNil)
			So(auditLog.Append("gate_reject", frame), ShouldBeNil)

			lines, readErr := readAuditLines(path)

			Convey("It should keep only the first line", func() {
				So(readErr, ShouldBeNil)
				So(lines, ShouldHaveLength, 1)
			})
		})

		Convey("When the reason changes", func() {
			So(auditLog.Append("gate_reject", frame), ShouldBeNil)
			changed := map[string]any{
				"symbol":   "VVV/EUR",
				"playbook": "trend",
				"reason":   "toxic_bluff_deny",
			}
			So(auditLog.Append("gate_reject", changed), ShouldBeNil)

			lines, readErr := readAuditLines(path)

			Convey("It should log both transitions", func() {
				So(readErr, ShouldBeNil)
				So(lines, ShouldHaveLength, 2)
			})
		})
	})
}

func TestAuditLogRotate(t *testing.T) {
	Convey("Given a tiny audit log rotation threshold", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 128, 3, time.Minute)
		So(err, ShouldBeNil)
		defer auditLog.Close()

		payload := map[string]any{
			"symbol": "BTC/EUR",
			"reason": string(make([]byte, 256)),
		}

		Convey("When writes exceed the byte cap", func() {
			for range 8 {
				So(auditLog.Append("entry", payload), ShouldBeNil)
			}

			_, statErr := os.Stat(path + ".1")

			Convey("It should rotate to a numbered backup", func() {
				So(statErr, ShouldBeNil)
			})
		})
	})
}

func readAuditLines(path string) ([]map[string]any, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]map[string]any, 0)

	for scanner.Scan() {
		record := map[string]any{}

		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}

		lines = append(lines, record)
	}

	return lines, scanner.Err()
}

func BenchmarkAuditLogAppend(b *testing.B) {
	path := b.TempDir() + "/audit.jsonl"
	auditLog, err := OpenAuditLog(path, defaultAuditMaxBytes, defaultAuditMaxFiles, time.Minute)

	if err != nil {
		b.Fatal(err)
	}

	defer auditLog.Close()

	frame := map[string]any{
		"symbol":   "BTC/EUR",
		"playbook": "trend",
		"reason":   "systemic_slump_wait",
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = auditLog.Append("gate_reject", frame)
	}
}
