package workbench

import "testing"

func TestCatalogName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"s3://symmtables/":   "symmtables",
		"s3://symmtables":    "symmtables",
		"s3://bucket/path/x": "bucket",
		"symmtables":         "symmtables",
	}

	for location, want := range cases {
		if got := catalogName(location); got != want {
			t.Fatalf("catalogName(%q) = %q, want %q", location, got, want)
		}
	}
}

func TestSanitizeStatement(t *testing.T) {
	t.Parallel()

	got := sanitizeStatement("SELECT FROM memory.main.v1 LIMIT 10")
	want := "SELECT NULL FROM memory.main.v1 LIMIT 10"

	if got != want {
		t.Fatalf("sanitizeStatement = %q, want %q", got, want)
	}

	unchanged := "SELECT 1 FROM dual"
	if sanitizeStatement(unchanged) != unchanged {
		t.Fatalf("sanitizeStatement mutated a valid select")
	}
}

func TestNested(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"MAP(VARCHAR, DOUBLE)", "STRUCT(a INTEGER)", "UNION(a INTEGER)", "LIST(INTEGER)", "VARCHAR[]", "INTEGER[]"} {
		if !nested(kind) {
			t.Fatalf("expected nested(%q)", kind)
		}
	}

	for _, kind := range []string{"VARCHAR", "DOUBLE", "BIGINT", "TIMESTAMP WITH TIME ZONE", "BOOLEAN"} {
		if nested(kind) {
			t.Fatalf("did not expect nested(%q)", kind)
		}
	}
}

func TestSelection(t *testing.T) {
	t.Parallel()

	if got := selection("mid", "DOUBLE"); got != `"mid"` {
		t.Fatalf("selection plain = %q", got)
	}

	got := selection("metrics", "MAP(VARCHAR, DOUBLE)")
	want := `to_json("metrics")::VARCHAR AS "metrics"`
	if got != want {
		t.Fatalf("selection nested = %q, want %q", got, want)
	}
}

func TestLiteralAndIdentifier(t *testing.T) {
	t.Parallel()

	if got := literal("a'b"); got != "'a''b'" {
		t.Fatalf("literal = %q", got)
	}

	if got := identifier(`a"b`); got != `"a""b"` {
		t.Fatalf("identifier = %q", got)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	warehouse := New()
	if warehouse == nil {
		t.Fatal("New returned nil")
	}

	if err := warehouse.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
