package kraken

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestNewClientPaper(t *testing.T) {
	Convey("Given paper trading config", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		client, err := NewClient(ctx, pool)

		Convey("It should construct a kraken client", func() {
			So(err, ShouldBeNil)
			So(client, ShouldNotBeNil)
			So(client.Close(), ShouldBeNil)
		})
	})
}

