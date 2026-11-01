package workbench

import (
	"regexp"
	"strings"
)

var selectWithoutSelection = regexp.MustCompile(`(?i)\bSELECT\s+FROM\b`)

func sanitizeStatement(statement string) string {
	return selectWithoutSelection.ReplaceAllString(statement, "SELECT NULL FROM")
}

func literal(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func identifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func nested(kind string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(kind))

	if strings.HasSuffix(normalized, "[]") || strings.HasSuffix(normalized, ")[]") {
		return true
	}

	for _, prefix := range []string{"MAP(", "STRUCT(", "UNION(", "LIST("} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}

	return false
}

func selection(name, kind string) string {
	quoted := identifier(name)

	if !nested(kind) {
		return quoted
	}

	return "to_json(" + quoted + ")::VARCHAR AS " + quoted
}
