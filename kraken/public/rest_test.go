package public

import (
	"context"
	"testing"

	"github.com/theapemachine/datura/dmt"
	. "github.com/smartystreets/goconvey/convey"
)

func restTestTree(t testing.TB) *dmt.Tree {
	if t != nil {
		t.Helper()
	}

	return dmt.NewTree("")
}

func TestNewRest(t *testing.T) {
	Convey("Given a parent context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tree := restTestTree(t)

		Convey("It should construct a REST client with the injected tree", func() {
			rest := NewRest(ctx, EndpointTypeTicker, tree)
			defer rest.Close()

			So(rest, ShouldNotBeNil)
			So(rest.client, ShouldNotBeNil)
			So(rest.endpoint, ShouldEqual, EndpointTypeTicker)
			So(rest.tree, ShouldEqual, tree)
		})
	})
}

func TestRestClose(t *testing.T) {
	Convey("Given a REST client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		tree := restTestTree(t)
		rest := NewRest(ctx, EndpointTypeTicker, tree)

		Convey("When closed", func() {
			err := rest.Close()
			cancel()

			Convey("It should cancel the context", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkNewRest(b *testing.B) {
	ctx := context.Background()
	tree := restTestTree(b)

	b.ReportAllocs()

	for b.Loop() {
		rest := NewRest(ctx, EndpointTypeTicker, tree)
		_ = rest.Close()
	}
}
