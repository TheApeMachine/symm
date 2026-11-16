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

			Convey("It returns six ledger views, three stoploss tapes, and two level3 views", func() {
				So(len(seeds), ShouldEqual, 11)
				So(seeds[0].name, ShouldEqual, "open_paper")
				So(seeds[1].name, ShouldEqual, "closed_paper")
				So(seeds[2].name, ShouldEqual, "open_real")
				So(seeds[3].name, ShouldEqual, "closed_real")
				So(seeds[4].name, ShouldEqual, "open_virtual")
				So(seeds[5].name, ShouldEqual, "closed_virtual")
				So(seeds[6].name, ShouldEqual, "stoploss_paper")
				So(seeds[7].name, ShouldEqual, "stoploss_real")
				So(seeds[8].name, ShouldEqual, "stoploss_virtual")
				So(seeds[9].name, ShouldEqual, "level3_frames")
				So(seeds[10].name, ShouldEqual, "level3_events")
			})
		})
	})
}

func TestStoplossView(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewCatalog("symm")

		Convey("When stoplossView builds SQL for stoploss_paper", func() {
			sql := catalog.stoplossView("stoploss_paper", "s3://symm/paper/stops/**")

			Convey("The CREATE VIEW reads Stoploss JSON with path-derived keys", func() {
				So(sql, ShouldStartWith, `CREATE OR REPLACE VIEW warehouse.main."stoploss_paper" AS SELECT `)
				So(sql, ShouldContainSubstring, "FROM read_json('s3://symm/paper/stops/**'")
				So(sql, ShouldContainSubstring, "filename=true")
				So(sql, ShouldContainSubstring, "AS noted_at_ns")
				So(sql, ShouldContainSubstring, "(PeakPnLPct - CurrentPnLPct) AS giveback_pct")
				So(sql, ShouldContainSubstring, "'TriggerReason': 'VARCHAR'")
			})
		})
	})
}

func TestEmptyStoplossView(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewCatalog("symm")

		Convey("When emptyStoplossView builds SQL", func() {
			sql := catalog.emptyStoplossView("stoploss_paper")

			Convey("It is a typed empty SELECT WHERE false", func() {
				So(sql, ShouldContainSubstring, `CREATE OR REPLACE VIEW warehouse.main."stoploss_paper" AS SELECT `)
				So(sql, ShouldContainSubstring, "CAST(NULL AS VARCHAR) AS symbol")
				So(sql, ShouldContainSubstring, "CAST(NULL AS UBIGINT) AS noted_at_ns")
				So(sql, ShouldContainSubstring, "CAST(NULL AS DOUBLE) AS giveback_pct")
				So(sql, ShouldEndWith, "WHERE false")
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

func BenchmarkNewCatalog(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewCatalog("symm")
	}
}

func BenchmarkSeeds(b *testing.B) {
	catalog := NewCatalog("symm")

	b.ReportAllocs()

	for b.Loop() {
		_ = catalog.seeds()
	}
}

func BenchmarkPositionView(b *testing.B) {
	catalog := NewCatalog("symm")

	b.ReportAllocs()

	for b.Loop() {
		_ = catalog.positionView("open_paper", "s3://symm/paper/open/**")
	}
}

func BenchmarkEmptyPositionView(b *testing.B) {
	catalog := NewCatalog("symm")

	b.ReportAllocs()

	for b.Loop() {
		_ = catalog.emptyPositionView("closed_paper")
	}
}
