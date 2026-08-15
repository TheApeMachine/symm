package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSystemSync(t *testing.T) {
	Convey("Given a system without a trader", t, func() {
		err := (&System{}).Sync(t.Context(), time.Time{})

		Convey("It should refuse to pretend the market was consumed", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSystemClose(t *testing.T) {
	Convey("Given resources recorded in acquisition order", t, func() {
		closed := make([]int, 0, 3)
		system := &System{closers: []func() error{
			func() error {
				closed = append(closed, 1)
				return nil
			},
			func() error {
				closed = append(closed, 2)
				return nil
			},
			func() error {
				closed = append(closed, 3)
				return nil
			},
		}}

		err := system.Close()

		Convey("It should release every resource in reverse acquisition order", func() {
			So(err, ShouldBeNil)
			So(closed, ShouldResemble, []int{3, 2, 1})
		})
	})
}
