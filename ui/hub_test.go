package ui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func TestHubMonigoDashboard(t *testing.T) {
	Convey("Given a ui hub with an enlarged read buffer", t, func() {
		viper.Set("ui.addr", "127.0.0.1:0")
		viper.Set("ui.read_buffer_size", 16*1024)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		defer pool.Close()

		hub := NewHub(ctx, pool)

		So(hub, ShouldNotBeNil)
		So(hub.app.Config().ReadBufferSize, ShouldEqual, 16*1024)

		t.Cleanup(func() {
			_ = hub.Close()
		})

		Convey("It should serve the MoniGo dashboard without 431", func() {
			request := httptest.NewRequest(http.MethodGet, "/monigo/", nil)
			request.Header.Set("Cookie", strings.Repeat("session=abcdef0123456789;", 512))

			response, err := hub.app.Test(request)

			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldNotEqual, http.StatusRequestHeaderFieldsTooLarge)
		})

		Convey("It should redirect bare /monigo to the dashboard root", func() {
			request := httptest.NewRequest(http.MethodGet, "/monigo", nil)

			response, err := hub.app.Test(request)

			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, http.StatusMovedPermanently)
			So(response.Header.Get("Location"), ShouldEqual, "/monigo/")
		})

		Convey("It should serve the MoniGo index page", func() {
			request := httptest.NewRequest(http.MethodGet, "/monigo/", nil)

			response, err := hub.app.Test(request)

			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, http.StatusOK)

			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()

			So(readErr, ShouldBeNil)
			So(string(body), ShouldContainSubstring, "<html")
			So(string(body), ShouldNotContainSubstring, "Could not load static/monigo")
		})
	})
}
