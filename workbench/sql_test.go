package workbench

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSanitizeStatement(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When SELECT has no projection", func() {
			out := sanitizeStatement("SELECT FROM closed_paper")

			Convey("NULL is inserted as the selection", func() {
				So(out, ShouldEqual, "SELECT NULL FROM closed_paper")
			})
		})

		Convey("When SELECT already has a projection", func() {
			out := sanitizeStatement("SELECT id FROM closed_paper")

			Convey("The statement is unchanged", func() {
				So(out, ShouldEqual, "SELECT id FROM closed_paper")
			})
		})

		Convey("When SELECT FROM is mixed case", func() {
			out := sanitizeStatement("select from t")

			Convey("NULL is inserted case-insensitively", func() {
				So(out, ShouldEqual, "SELECT NULL FROM t")
			})
		})
	})
}

func TestLiteral(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When literal wraps a plain string", func() {
			Convey("It is single-quoted", func() {
				So(literal("s3://symm/paper"), ShouldEqual, "'s3://symm/paper'")
			})
		})

		Convey("When the value contains a single quote", func() {
			Convey("The quote is escaped by doubling", func() {
				So(literal("O'Brien"), ShouldEqual, "'O''Brien'")
			})
		})

		Convey("When the value is empty", func() {
			Convey("It is an empty quoted string", func() {
				So(literal(""), ShouldEqual, "''")
			})
		})
	})
}

func TestIdentifier(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When identifier wraps a plain name", func() {
			Convey("It is double-quoted", func() {
				So(identifier("open_paper"), ShouldEqual, `"open_paper"`)
			})
		})

		Convey("When the name contains a double quote", func() {
			Convey("The quote is escaped by doubling", func() {
				So(identifier(`a"b`), ShouldEqual, `"a""b"`)
			})
		})

		Convey("When the name is empty", func() {
			Convey("It is an empty quoted identifier", func() {
				So(identifier(""), ShouldEqual, `""`)
			})
		})
	})
}

func TestNested(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the kind is a plain scalar", func() {
			Convey("nested is false", func() {
				So(nested("VARCHAR"), ShouldBeFalse)
				So(nested("BIGINT"), ShouldBeFalse)
				So(nested(""), ShouldBeFalse)
			})
		})

		Convey("When the kind ends with []", func() {
			Convey("nested is true", func() {
				So(nested("VARCHAR[]"), ShouldBeTrue)
				So(nested("INTEGER[]"), ShouldBeTrue)
			})
		})

		Convey("When the kind ends with )[]", func() {
			Convey("nested is true", func() {
				So(nested("STRUCT(a INTEGER)[]"), ShouldBeTrue)
			})
		})

		Convey("When the kind starts with a nested constructor", func() {
			Convey("nested is true for MAP STRUCT UNION LIST", func() {
				So(nested("MAP(VARCHAR, INTEGER)"), ShouldBeTrue)
				So(nested("STRUCT(a INTEGER)"), ShouldBeTrue)
				So(nested("UNION(a INTEGER)"), ShouldBeTrue)
				So(nested("LIST(INTEGER)"), ShouldBeTrue)
			})
		})

		Convey("When the kind is mixed case with space", func() {
			Convey("Normalization still detects nested", func() {
				So(nested("  map(varchar, integer)  "), ShouldBeTrue)
				So(nested("  varchar[]  "), ShouldBeTrue)
			})
		})
	})
}

func TestSelection(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the column is a scalar", func() {
			out := selection("id", "VARCHAR")

			Convey("It is a quoted identifier only", func() {
				So(out, ShouldEqual, `"id"`)
			})
		})

		Convey("When the column is nested", func() {
			out := selection("fills", "STRUCT(side VARCHAR)[]")

			Convey("It is wrapped in to_json::VARCHAR AS", func() {
				So(out, ShouldEqual, `to_json("fills")::VARCHAR AS "fills"`)
			})
		})

		Convey("When the nested name needs quote escaping", func() {
			out := selection(`a"b`, "MAP(VARCHAR, INTEGER)")

			Convey("Both identifier sites escape quotes", func() {
				So(out, ShouldEqual, `to_json("a""b")::VARCHAR AS "a""b"`)
			})
		})
	})
}

func BenchmarkSanitizeStatement(b *testing.B) {
	stmt := "SELECT id FROM closed_paper"

	b.ReportAllocs()

	for b.Loop() {
		_ = sanitizeStatement(stmt)
	}
}

func BenchmarkLiteral(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = literal("s3://symm/paper")
	}
}

func BenchmarkIdentifier(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = identifier("open_paper")
	}
}

func BenchmarkNested(b *testing.B) {
	kind := "STRUCT(a INTEGER)[]"

	b.ReportAllocs()

	for b.Loop() {
		_ = nested(kind)
	}
}

func BenchmarkSelection(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = selection("fills", "STRUCT(side VARCHAR)[]")
	}
}
