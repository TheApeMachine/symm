package workbench

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewCatalog(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewCatalog is called", func() {
			catalog := NewCatalog("symm")

			Convey("The bucket is stored", func() {
				So(catalog, ShouldNotBeNil)
				So(catalog.bucket, ShouldEqual, "symm")
			})
		})
	})
}

func TestSeeds(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewCatalog("symm")

		Convey("When seeds is called", func() {
			seeds := catalog.seeds()

			Convey("It returns six ledger views plus two level3 views", func() {
				So(len(seeds), ShouldEqual, 8)
				So(seeds[0].name, ShouldEqual, "open_paper")
				So(seeds[1].name, ShouldEqual, "closed_paper")
				So(seeds[2].name, ShouldEqual, "open_real")
				So(seeds[3].name, ShouldEqual, "closed_real")
				So(seeds[4].name, ShouldEqual, "open_virtual")
				So(seeds[5].name, ShouldEqual, "closed_virtual")
				So(seeds[6].name, ShouldEqual, "level3_frames")
				So(seeds[7].name, ShouldEqual, "level3_events")
			})
		})
	})
}

func TestPositionView(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewCatalog("symm")

		Convey("When positionView builds SQL for open_paper", func() {
			sql := catalog.positionView("open_paper", "s3://symm/paper/open/**")

			Convey("The CREATE VIEW targets warehouse.main.\"open_paper\"", func() {
				So(sql, ShouldStartWith, `CREATE OR REPLACE VIEW warehouse.main."open_paper" AS SELECT `)
				So(sql, ShouldContainSubstring, "FROM read_json('s3://symm/paper/open/**'")
				So(sql, ShouldContainSubstring, "json_extract_string(position, '$.id') AS id")
				So(sql, ShouldContainSubstring, "'position': 'JSON'")
			})
		})
	})
}

func TestEmptyPositionView(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewCatalog("symm")

		Convey("When emptyPositionView builds SQL", func() {
			sql := catalog.emptyPositionView("closed_paper")

			Convey("It is a typed empty SELECT WHERE false", func() {
				So(sql, ShouldEqual,
					`CREATE OR REPLACE VIEW warehouse.main."closed_paper" AS SELECT `+
						"CAST(NULL AS UBIGINT) AS epoch, "+
						"CAST(NULL AS UBIGINT) AS tick, "+
						"CAST(NULL AS UBIGINT) AS timestamp, "+
						"CAST(NULL AS VARCHAR) AS id, "+
						"CAST(NULL AS VARCHAR) AS symbol, "+
						"CAST(NULL AS VARCHAR) AS book, "+
						"CAST(NULL AS VARCHAR) AS entry_path, "+
						"CAST(NULL AS VARCHAR) AS exit_path, "+
						"CAST(NULL AS VARCHAR) AS exit_trigger, "+
						"CAST(NULL AS VARCHAR) AS qty, "+
						"CAST(NULL AS VARCHAR) AS entry_price, "+
						"CAST(NULL AS VARCHAR) AS exit_price, "+
						"CAST(NULL AS VARCHAR) AS pnl, "+
						"CAST(NULL AS DOUBLE) AS pnl_pct, "+
						"CAST(NULL AS VARCHAR) AS opened_at, "+
						"CAST(NULL AS VARCHAR) AS closed_at, "+
						"CAST(NULL AS VARCHAR) AS fills, "+
						"CAST(NULL AS VARCHAR) AS hold_paths WHERE false",
				)
			})
		})
	})
}
