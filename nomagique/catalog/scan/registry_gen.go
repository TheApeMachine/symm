package scan

import (
	"github.com/theapemachine/symm/nomagique/catalog"
)

/*
GenerateRegistry generates metadata for the Flume catalog.
The Cap'n Proto runtime uses schemas directly without generated shadow execution glue.
*/
func GenerateRegistry(schemas map[string]catalog.Schema) error {
	return nil
}
