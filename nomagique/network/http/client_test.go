package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* TestHTTPClientWrite exercises actual HTTP transport, explicit inputs and failures. */
func TestHTTPClientWrite(t *testing.T) {
	Convey("Given a loopback HTTP endpoint", t, func() {
		endpoint := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/error" {
				response.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if request.Method != http.MethodPost || request.Header.Get("X-Fixture") != "graph" {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, err := io.Copy(response, request.Body); err != nil {
				t.Error(err)
			}
		}))
		defer endpoint.Close()
		for _, fixture := range []struct {
			name, path, method string
			valid              bool
		}{
			{"declared headers and payload", "/echo", "POST", true},
			{"server error remains an error", "/error", "POST", false},
			{"missing method is rejected", "/echo", "", false},
		} {
			Convey(fixture.name, func() {
				ctx := context.Background()
				client := HTTPClient_ServerToClient(NewHTTPClient())
				defer client.Release()
				So(client.Write(ctx, func(args HTTPClient_write_Params) error {
					if err := args.SetUrl(endpoint.URL + fixture.path); err != nil {
						return err
					}
					if err := args.SetMethod(fixture.method); err != nil {
						return err
					}
					headers, err := args.NewHeaders(1)
					if err != nil {
						return err
					}
					if err := headers.At(0).SetName("X-Fixture"); err != nil {
						return err
					}
					if err := headers.At(0).SetValue("graph"); err != nil {
						return err
					}
					return args.SetBody([]byte("exact payload\x00"))
				}), ShouldBeNil)
				err := client.WaitStreaming()
				if !fixture.valid {
					So(err, ShouldNotBeNil)
					return
				}
				So(err, ShouldBeNil)
				future, release := client.Done(ctx, nil)
				defer release()
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Status(), ShouldEqual, http.StatusOK)
				payload, err := result.Out()
				So(err, ShouldBeNil)
				So(string(payload), ShouldEqual, "exact payload\x00")
			})
		}
	})
}
