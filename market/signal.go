package market

import "github.com/theapemachine/datura"

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(*datura.Artifact) *datura.Artifact
	IngestRoles() []string
	Close() error
}

/*
IngestScopes returns Kraken ingest scopes to replay for one measurement query.
Live websocket frames use type "update"; book also publishes "snapshot" rows.
*/
func IngestScopes(scope string) []string {
	if scope == "" {
		return nil
	}

	if scope == "snapshot" {
		return []string{scope}
	}

	return []string{scope, "snapshot"}
}
