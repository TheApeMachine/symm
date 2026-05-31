package broker

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRouterPublish(t *testing.T) {
	Convey("Given a router publisher", t, func() {
		var published any
		router := NewRouter(func(value any) error { published = value; return nil })

		err := router.Publish(map[string]string{"method": "add_order"})

		Convey("It should forward frames to the publisher", func() {
			So(err, ShouldBeNil)
			So(published, ShouldNotBeNil)
		})
	})

	Convey("Given a publisher that fails", t, func() {
		router := NewRouter(func(any) error { return fmt.Errorf("publish failed") })

		Convey("It should propagate the publish error", func() {
			So(router.Publish("frame"), ShouldNotBeNil)
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
	router := NewRouter(func(any) error { return nil })

	for b.Loop() {
		_ = router.Publish("frame")
	}
}
