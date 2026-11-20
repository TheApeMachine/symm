package network

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRestPath(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When RestPath is given a full URL", func() {
			path := RestPath("https://api.kraken.com/0/private/Balance")

			Convey("It returns the URL path", func() {
				So(path, ShouldEqual, "/0/private/Balance")
			})
		})

		Convey("When RestPath is given a bare path", func() {
			path := RestPath("/0/private/TradeBalance")

			Convey("It returns the path unchanged", func() {
				So(path, ShouldEqual, "/0/private/TradeBalance")
			})
		})

		Convey("When RestPath is given an empty string", func() {
			path := RestPath("")

			Convey("It returns empty", func() {
				So(path, ShouldEqual, "")
			})
		})
	})
}

func TestNewVirtual(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewVirtual is called", func() {
			virtual := NewVirtual(context.Background())

			Convey("It wires a queue and notify channel", func() {
				So(virtual, ShouldNotBeNil)
				So(virtual.queue, ShouldNotBeNil)
				So(virtual.notify, ShouldNotBeNil)
				So(virtual.Close(), ShouldBeNil)
			})
		})
	})
}

func TestVirtualClose(t *testing.T) {
	Convey("Setup", t, func() {
		virtual := NewVirtual(context.Background())

		Convey("When Close is called", func() {
			err := virtual.Close()

			Convey("ctx is cancelled and Read unblocks", func() {
				So(err, ShouldBeNil)
				_, readErr := virtual.Read()
				So(readErr, ShouldNotBeNil)
				So(readErr.Error(), ShouldContainSubstring, "context canceled")
			})
		})
	})
}

func TestVirtualWrite(t *testing.T) {
	Convey("Setup", t, func() {
		virtual := NewVirtual(context.Background())
		defer virtual.Close()

		Convey("When Write is given invalid JSON", func() {
			err := virtual.Write([]byte(`{`))

			Convey("It rejects the frame", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to unmarshal paper request")
			})
		})

		Convey("When Write has no mapped command", func() {
			err := virtual.Write([]byte(`{"method":"ping"}`))

			Convey("It reports no virtual command", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[kraken.Virtual.Write] no virtual command")
			})
		})
	})
}

func TestVirtualPost(t *testing.T) {
	Convey("Setup", t, func() {
		virtual := NewVirtual(context.Background())
		defer virtual.Close()

		Convey("When Post is given an unsupported path", func() {
			raw, err := virtual.Post("/0/public/Time", nil)

			Convey("It rejects the path", func() {
				So(raw, ShouldBeNil)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "[kraken.Virtual.Post] unsupported path")
			})
		})
	})
}

func BenchmarkRestPath(b *testing.B) {
	const path = "https://api.kraken.com/0/private/Balance"

	b.ReportAllocs()

	for b.Loop() {
		_ = RestPath(path)
	}
}

func BenchmarkNewVirtual(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		virtual := NewVirtual(ctx)
		_ = virtual.Close()
	}
}

func BenchmarkVirtualClose(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		virtual := NewVirtual(ctx)
		_ = virtual.Close()
	}
}

func BenchmarkVirtualWrite(b *testing.B) {
	virtual := NewVirtual(context.Background())
	defer virtual.Close()
	payload := []byte(`{"method":"ping"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = virtual.Write(payload)
	}
}

func BenchmarkVirtualPost(b *testing.B) {
	virtual := NewVirtual(context.Background())
	defer virtual.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = virtual.Post("/0/public/Time", nil)
	}
}
