package market

import (
	"github.com/theapemachine/datura"
)

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

/*
IngestFrameMatches reports whether artifact is a live Kraken frame for role.
Websocket ingest keys frames by timestamp only; role and scope live on the artifact.
*/
func IngestFrameMatches(artifact *datura.Artifact, role string) bool {
	if artifact == nil || role == "" {
		return false
	}

	artifactRole, roleErr := artifact.Role()

	if roleErr != nil || artifactRole != role {
		return false
	}

	channel := datura.Peek[string](artifact, "channel")

	if channel != "" && channel != role {
		return false
	}

	scope, scopeErr := artifact.Scope()

	if scopeErr != nil {
		return false
	}

	for _, allowed := range IngestScopes("update") {
		if scope == allowed {
			return true
		}
	}

	return false
}
