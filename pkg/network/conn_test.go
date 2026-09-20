package network

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRead(t *testing.T) {
	Convey("Given a UniConn with no transport", t, func() {
		conn := NewUniConn(t.Context())
		So(conn, ShouldNotBeNil)

		Convey("Ready returns an error", func() {
			err := conn.Ready()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "transport")
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given a UniConn with no transport", t, func() {
		conn := NewUniConn(t.Context())
		So(conn, ShouldNotBeNil)

		Convey("Ready returns an error", func() {
			err := conn.Ready()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "transport")
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a UniConn with no transport", t, func() {
		conn := NewUniConn(t.Context())
		So(conn, ShouldNotBeNil)

		Convey("Close returns nil", func() {
			err := conn.Close()
			So(err, ShouldBeNil)
		})
	})
}

func TestSetActiveTransport(t *testing.T) {
	Convey("Given a UniConn with two transports", t, func() {
		first := newStubManagedTransport()
		second := newStubManagedTransport()
		conn := NewUniConn(
			t.Context(),
			UniConnWithTransport(IPCType, first),
			UniConnWithTransport(UDPType, second),
		)

		So(conn.activeType, ShouldEqual, IPCType)
		So(conn.SetActiveTransport(UDPType), ShouldBeNil)
		So(conn.activeType, ShouldEqual, UDPType)
		So(conn.active, ShouldEqual, second)
	})

	Convey("Given SetActiveTransport for a missing registration", t, func() {
		conn := NewUniConn(
			t.Context(),
			UniConnWithTransport(IPCType, newStubManagedTransport()),
		)
		So(conn.SetActiveTransport(QUICType), ShouldEqual, ErrNoTransport)
	})
}

func TestEnsureReady(t *testing.T) {
	Convey("Given a UniConn with no transport", t, func() {
		conn := NewUniConn(t.Context())
		So(conn, ShouldNotBeNil)

		Convey("ensureReady returns an error", func() {
			err := conn.ensureReady()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "transport")
		})
	})
}

func BenchmarkRead(b *testing.B) {
	conn := NewUniConn(b.Context())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Read(nil)
	}
}

func BenchmarkWrite(b *testing.B) {
	conn := NewUniConn(b.Context())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Write(nil)
	}
}

func BenchmarkSetActiveTransport(b *testing.B) {
	first := newStubManagedTransport()
	second := newStubManagedTransport()
	conn := NewUniConn(
		b.Context(),
		UniConnWithTransport(IPCType, first),
		UniConnWithTransport(UDPType, second),
	)
	b.ResetTimer()
	for b.Loop() {
		_ = conn.SetActiveTransport(UDPType)
		_ = conn.SetActiveTransport(IPCType)
	}
}

type stubManagedTransport struct {}
func newStubManagedTransport() ManagedTransport { return &stubManagedTransport{} }
func (s *stubManagedTransport) Read(p []byte) (n int, err error) { return 0, nil }
func (s *stubManagedTransport) Write(p []byte) (n int, err error) { return len(p), nil }
func (s *stubManagedTransport) Close() error { return nil }
func (s *stubManagedTransport) Traits() TransportTraits { return TransportTraits{} }
func (s *stubManagedTransport) Status() TransportStatus { return TransportStatus{} }

