package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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
