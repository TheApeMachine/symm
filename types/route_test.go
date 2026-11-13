package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoute(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Route()
		Reset(func() { SetRoute(prev) })

		Convey("When SetRoute stores longtail", func() {
			SetRoute("longtail")

			Convey("Route returns longtail", func() {
				So(Route(), ShouldEqual, "longtail")
			})
		})
	})
}

func TestSetRoute(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Route()
		Reset(func() { SetRoute(prev) })

		Convey("When SetRoute trims a leading slash", func() {
			SetRoute("/learning")

			Convey("Route stores the bare surface name", func() {
				So(Route(), ShouldEqual, "learning")
			})
		})

		Convey("When SetRoute receives empty input", func() {
			SetRoute("")

			Convey("Route falls back to dashboard", func() {
				So(Route(), ShouldEqual, "dashboard")
			})
		})
	})
}

func TestWantsGrid(t *testing.T) {
	Convey("Setup", t, func() {
		prev := Route()
		Reset(func() { SetRoute(prev) })

		Convey("When the surface is dashboard", func() {
			SetRoute("dashboard")

			Convey("WantsGrid is true", func() {
				So(WantsGrid(), ShouldBeTrue)
			})
		})

		Convey("When the surface is learning", func() {
			SetRoute("learning")

			Convey("WantsGrid is true", func() {
				So(WantsGrid(), ShouldBeTrue)
			})
		})

		Convey("When the surface is longtail", func() {
			SetRoute("longtail")

			Convey("WantsGrid is false", func() {
				So(WantsGrid(), ShouldBeFalse)
			})
		})

		Convey("When the surface is workbench", func() {
			SetRoute("workbench")

			Convey("WantsGrid is false", func() {
				So(WantsGrid(), ShouldBeFalse)
			})
		})
	})
}

func BenchmarkRoute(b *testing.B) {
	prev := Route()
	b.Cleanup(func() { SetRoute(prev) })
	SetRoute("longtail")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = Route()
	}
}

func BenchmarkSetRoute(b *testing.B) {
	prev := Route()
	b.Cleanup(func() { SetRoute(prev) })

	b.ReportAllocs()

	for b.Loop() {
		SetRoute("/learning")
	}
}

func BenchmarkWantsGrid(b *testing.B) {
	prev := Route()
	b.Cleanup(func() { SetRoute(prev) })
	SetRoute("dashboard")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = WantsGrid()
	}
}
