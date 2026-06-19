package signal

import (
	"io"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
)

/*
NewTestTree returns an isolated in-memory tree for unit tests.
*/
func NewTestTree() *dmt.Tree {
	return dmt.NewTree("")
}

/*
InsertTreeArtifact stores a marshaled artifact row at its Prefix key.
*/
func InsertTreeArtifact(tree *dmt.Tree, artifact *datura.Artifact) {
	if tree == nil || artifact == nil {
		return
	}

	wire, err := artifact.Message().Marshal()

	if err != nil || len(wire) == 0 {
		return
	}

	tree.Insert(artifact.Prefix(), wire)
}

var ingestRoles = []string{"book", "trade", "ticker", "order", "features"}

/*
ReplayScopeIngest runs the measurement query against every ingest row for scope.
*/
func ReplayScopeIngest(
	tree *dmt.Tree,
	scope string,
	query *datura.Artifact,
	algo io.ReadWriter,
) bool {
	if tree == nil || query == nil || algo == nil || scope == "" {
		return false
	}

	replayed := false

	for _, role := range ingestRoles {
		seek := datura.Acquire("trader", datura.Artifact_Type_json)
		seek.WithRole(role)
		seek.WithScope(scope)

		for stored := range tree.Seek(seek.Prefix()) {
			transport.Copy(query, stored)
			errnie.Error(transport.NewFlipFlop(query, algo))
			replayed = true
		}

		seek.Release()
	}

	return replayed
}

/*
PublishMeasurement indexes a classifier output under measurement/<scope>/<origin>.
*/
func PublishMeasurement(tree *dmt.Tree, origin string, query *datura.Artifact) {
	if tree == nil || query == nil || origin == "" {
		return
	}

	if datura.Peek[int](query, "classifier", "category") <= 0 {
		return
	}

	if datura.Peek[float64](query, "classifier", "confidence") <= 0 {
		return
	}

	if existing, _ := query.Origin(); existing == "" {
		_ = query.SetOrigin(origin)
	}

	InsertMeasurement(tree, query)
}

/*
InsertMeasurement indexes a publishable classifier measurement in the shared tree.
*/
func InsertMeasurement(tree *dmt.Tree, artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	if datura.Peek[int](artifact, "classifier", "category") <= 0 {
		return
	}

	if datura.Peek[float64](artifact, "classifier", "confidence") <= 0 {
		return
	}

	artifact.WithRole("measurement")
	InsertTreeArtifact(tree, artifact)
}
