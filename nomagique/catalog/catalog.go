package catalog

import (
	_ "embed"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

//go:embed primitives.json
var PrimitivesJSON []byte

/*
Load returns the map of all primitive schemas from the authoritative embedded catalog.
*/
func Load() (map[string]Schema, error) {
	var schemas map[string]Schema

	if err := sonic.Unmarshal(PrimitivesJSON, &schemas); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"catalog: failed to unmarshal embedded primitives.json",
			err,
		))
	}

	return schemas, nil
}
