package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestCovarianceMatrix(t *testing.T) {
	Convey("Given a CovarianceMatrix server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewCovarianceMatrix(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.CovarianceMatrix_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.CovarianceMatrix_write_Params) error {
				params.SetRows(3)
				params.SetCols(2)
				listData, err := params.NewData(6)

				if err != nil {
					return err
				}
				listData.Set(0, 1.0)
				listData.Set(1, 2.0)
				listData.Set(2, 3.0)
				listData.Set(3, 4.0)
				listData.Set(4, 5.0)
				listData.Set(5, 6.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
