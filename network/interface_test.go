package network

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubClient struct{}

func (stubClient) Read(...string) ([]byte, error) { return nil, nil }
func (stubClient) Write([]byte) error             { return nil }
func (stubClient) Post(string, any) ([]byte, error) {
	return nil, nil
}
func (stubClient) Watch(string, any, json.Unmarshaler) {}

func TestClient(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a type satisfies Client", func() {
			var client Client = stubClient{}

			Convey("Read, Write, Post, and Watch are callable", func() {
				raw, err := client.Read("k")
				So(err, ShouldBeNil)
				So(raw, ShouldBeNil)
				So(client.Write(nil), ShouldBeNil)
				post, err := client.Post("/x", nil)
				So(err, ShouldBeNil)
				So(post, ShouldBeNil)
				So(func() { client.Watch("/x", nil, nil) }, ShouldNotPanic)
			})
		})
	})
}

func BenchmarkClient(b *testing.B) {
	client := stubClient{}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = client.Read("k")
		_ = client.Write(nil)
		_, _ = client.Post("/x", nil)
		client.Watch("/x", nil, nil)
	}
}
