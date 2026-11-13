package workbench

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewServer(t *testing.T) {
	Convey("Setup", t, func() {
		warehouse := &Warehouse{}

		Convey("When NewServer is called", func() {
			server := NewServer(warehouse)

			Convey("The warehouse and fiber app are wired", func() {
				So(server, ShouldNotBeNil)
				So(server.warehouse, ShouldEqual, warehouse)
				So(server.app, ShouldNotBeNil)
			})
		})
	})
}

func TestListen(t *testing.T) {
	Convey("Setup", t, func() {
		prev := system.Cfg.Workbench.Addr
		system.Cfg.Workbench.Addr = "127.0.0.1:0"
		Reset(func() { system.Cfg.Workbench.Addr = prev })

		server := NewServer(nil)
		errCh := make(chan error, 1)

		Convey("When Listen binds an ephemeral port", func() {
			go func() {
				errCh <- server.Listen()
			}()

			time.Sleep(50 * time.Millisecond)
			shutdownErr := server.app.Shutdown()
			listenErr := <-errCh

			Convey("Shutdown stops Listen cleanly", func() {
				So(shutdownErr, ShouldBeNil)
				So(listenErr, ShouldBeNil)
			})
		})
	})
}

func TestQuery(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the body is not JSON", func() {
			server := NewServer(nil)
			req := httptest.NewRequest(http.MethodPost, "/workbench/query", bytes.NewBufferString("not-json"))
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)

			Convey("It returns bad request", func() {
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, fiber.StatusBadRequest)
			})
		})

		Convey("When SQL is empty", func() {
			server := NewServer(nil)
			req := httptest.NewRequest(http.MethodPost, "/workbench/query", bytes.NewBufferString(`{"sql":""}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)
			body, _ := io.ReadAll(resp.Body)

			Convey("It returns bad request for an empty query", func() {
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, fiber.StatusBadRequest)
				So(string(body), ShouldEqual, "query is empty")
			})
		})

		Convey("When SQL selects from the local warehouse", func() {
			warehouse := local(t)
			server := NewServer(warehouse)
			req := httptest.NewRequest(
				http.MethodPost,
				"/workbench/query",
				bytes.NewBufferString(`{"sql":"SELECT id FROM closed_paper"}`),
			)
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)
			body, _ := io.ReadAll(resp.Body)

			Convey("It returns an Arrow IPC stream", func() {
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, fiber.StatusOK)
				So(resp.Header.Get(fiber.HeaderContentType), ShouldEqual, arrowStream)
				So(len(body), ShouldEqual, 312)
			})
		})

		Convey("When SQL is invalid", func() {
			warehouse := local(t)
			server := NewServer(warehouse)
			req := httptest.NewRequest(
				http.MethodPost,
				"/workbench/query",
				bytes.NewBufferString(`{"sql":"SELECT FROM nowhere_table"}`),
			)
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.app.Test(req)

			Convey("It returns bad request", func() {
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, fiber.StatusBadRequest)
			})
		})
	})
}

func BenchmarkNewServer(b *testing.B) {
	warehouse := &Warehouse{}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewServer(warehouse)
	}
}

func BenchmarkListen(b *testing.B) {
	prev := system.Cfg.Workbench.Addr
	system.Cfg.Workbench.Addr = "127.0.0.1:0"
	defer func() { system.Cfg.Workbench.Addr = prev }()

	b.ReportAllocs()

	for b.Loop() {
		server := NewServer(nil)
		errCh := make(chan error, 1)

		go func() {
			errCh <- server.Listen()
		}()

		time.Sleep(20 * time.Millisecond)
		_ = server.app.Shutdown()
		<-errCh
	}
}

func BenchmarkQuery(b *testing.B) {
	warehouse := local(b)
	server := NewServer(warehouse)
	payload := `{"sql":"SELECT id FROM closed_paper"}`

	b.ReportAllocs()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/workbench/query", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.app.Test(req)
		if err == nil && resp != nil {
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
	}
}
