package replay

import (
	"encoding/binary"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func diagnosticSegment(identity, index, count uint32, payload string) []byte {
	segment := make([]byte, 16+len(payload))
	copy(segment, "SFD1")
	binary.LittleEndian.PutUint32(segment[4:8], identity)
	binary.LittleEndian.PutUint32(segment[8:12], index)
	binary.LittleEndian.PutUint32(segment[12:16], count)
	copy(segment[16:], payload)
	return segment
}

func TestDiagnosticRecordAccept(t *testing.T) {
	Convey("Diagnostics records preserve segment identity under unordered delivery", t, func() {
		var record diagnosticRecord
		payload, err := record.Accept(diagnosticSegment(1, 1, 2, "second"))
		So(err, ShouldBeNil)
		So(payload, ShouldBeNil)
		payload, err = record.Accept(diagnosticSegment(1, 0, 2, "first"))
		So(err, ShouldBeNil)
		So(string(payload), ShouldEqual, "firstsecond")

		Convey("Duplicate segments cannot produce another complete record", func() {
			payload, err = record.Accept(diagnosticSegment(1, 0, 2, "first"))
			So(err, ShouldBeNil)
			So(payload, ShouldBeNil)
		})
		Convey("Newer complete records supersede an incomplete older frame", func() {
			payload, err = record.Accept(diagnosticSegment(2, 0, 2, "old"))
			So(err, ShouldBeNil)
			So(payload, ShouldBeNil)
			payload, err = record.Accept(diagnosticSegment(3, 0, 1, "new"))
			So(err, ShouldBeNil)
			So(string(payload), ShouldEqual, "new")
			payload, err = record.Accept(diagnosticSegment(2, 1, 2, "late"))
			So(err, ShouldBeNil)
			So(payload, ShouldBeNil)
		})
		Convey("Malformed segment headers fail explicitly", func() {
			_, err = record.Accept([]byte("bad"))
			So(err, ShouldNotBeNil)
			_, err = record.Accept(diagnosticSegment(4, 1, 1, "bad"))
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkDiagnosticRecordAccept(b *testing.B) {
	// A two-segment record exercises the actual reassembly path.
	first, second := diagnosticSegment(1, 0, 2, "first"), diagnosticSegment(1, 1, 2, "second")
	b.ReportAllocs()

	for b.Loop() {
		var record diagnosticRecord
		if _, err := record.Accept(second); err != nil {
			b.Fatal(err)
		}

		if _, err := record.Accept(first); err != nil {
			b.Fatal(err)
		}
	}
}
