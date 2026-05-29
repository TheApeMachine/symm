package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRouterPublish(t *testing.T) {
	Convey("Given a router publisher", t, func() {
		var published any
		router := NewRouter(func(value any) { published = value })

		err := router.Publish(map[string]string{"method": "add_order"})

		Convey("It should forward frames to the publisher", func() {
			So(err, ShouldBeNil)
			So(published, ShouldNotBeNil)
		})
	})

	Convey("Given a nil router", t, func() {
		var router *Router

		Convey("It should reject publish", func() {
			So(router.Publish("frame"), ShouldNotBeNil)
		})
	})
}

func BenchmarkRouterPublish(b *testing.B) {
	router := NewRouter(func(any) {})

	for b.Loop() {
		_ = router.Publish("frame")
	}
}
