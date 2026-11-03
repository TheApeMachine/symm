package kraken_test

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func withPaper(f func(*kraken.Paper)) func() {
	return func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f(kraken.NewPaper(ctx, venue.NewSimulator()))
	}
}

func TestPaperWrite(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *kraken.Paper) {
		cases := []struct {
			name    string
			message []byte
			wantErr bool
		}{
			{"garbage JSON", []byte("{"), true},
			{"unknown method", mustJSON(map[string]any{"method": "ping"}), true},
			{
				"unknown subscribe channel",
				mustJSON(map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "ticker"},
				}),
				true,
			},
			{
				"subscribe without params",
				mustJSON(map[string]any{"method": "subscribe"}),
				true,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				err := paper.Write(tc.message)
				So(err != nil, ShouldEqual, tc.wantErr)
			})
		}
	}))
}

func TestPaperPost(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *kraken.Paper) {
		Convey("When the path is unknown", func() {
			raw, err := paper.Post("/0/private/OpenOrders", nil)
			So(err, ShouldNotBeNil)
			So(raw, ShouldBeNil)
		})

		Convey("When the context is cancelled before Balance Post", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = kraken.NewPaper(ctx, venue.NewSimulator())
			cancel()

			raw, err := paper.Post("/0/private/Balance", nil)
			So(err, ShouldNotBeNil)
			So(raw, ShouldBeNil)
		})
	}))
}

func TestPaperRead(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *kraken.Paper) {
		Convey("When the context is cancelled with an empty queue", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = kraken.NewPaper(ctx, nil)
			cancel()

			_, err := paper.Read()
			So(err, ShouldNotBeNil)
		})
	}))
}

func mustJSON(value map[string]any) []byte {
	buf, err := sonic.Marshal(value)
	if err != nil {
		panic(err)
	}
	return buf
}
