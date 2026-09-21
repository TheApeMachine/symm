package ui

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUIComponentServer(t *testing.T) {
	Convey("Given a UIComponent capability", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewUIComponent()
		So(server, ShouldNotBeNil)

		client := UIComponent_ServerToClient(server)
		defer client.Release()

		Convey("When it is written a component name, className and props", func() {
			err := client.Write(ctx, func(params UIComponent_write_Params) error {
				if err := params.SetName("Panel"); err != nil {
					return err
				}

				if err := params.SetClassName("min-h-0 flex-1"); err != nil {
					return err
				}

				return params.SetPropsJson(`{"variant":"sunken"}`)
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)

			name, err := out.Name()
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "Panel")

			className, err := out.ClassName()
			So(err, ShouldBeNil)
			So(className, ShouldEqual, "min-h-0 flex-1")

			props, err := out.PropsJson()
			So(err, ShouldBeNil)
			So(props, ShouldEqual, `{"variant":"sunken"}`)
		})

		Convey("When a second evaluation follows the first", func() {
			err := client.Write(ctx, func(params UIComponent_write_Params) error {
				return params.SetName("Badge")
			})
			So(err, ShouldBeNil)

			first, releaseFirst := client.Done(ctx, nil)
			_, err = first.Struct()
			So(err, ShouldBeNil)
			releaseFirst()

			err = client.Write(ctx, func(params UIComponent_write_Params) error {
				return params.SetName("Meter")
			})
			So(err, ShouldBeNil)

			second, releaseSecond := client.Done(ctx, nil)
			defer releaseSecond()

			results, err := second.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)

			Convey("Then the result carries no state from the previous evaluation", func() {
				name, err := out.Name()
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "Meter")
			})
		})
	})
}

func TestUIRouteServer(t *testing.T) {
	Convey("Given a UIRoute capability", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewUIRoute()
		So(server, ShouldNotBeNil)

		client := UIRoute_ServerToClient(server)
		defer client.Release()

		Convey("When it is written a path and a title", func() {
			err := client.Write(ctx, func(params UIRoute_write_Params) error {
				if err := params.SetPath("/index"); err != nil {
					return err
				}

				return params.SetTitle("Market Overview")
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)

			path, err := out.Path()
			So(err, ShouldBeNil)
			So(path, ShouldEqual, "/index")

			title, err := out.Title()
			So(err, ShouldBeNil)
			So(title, ShouldEqual, "Market Overview")
		})
	})
}
