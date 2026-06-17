package signal

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
InsertTreeArtifact stores a marshaled artifact row at its Prefix key.
*/
func InsertTreeArtifact(tree *dmt.Tree, artifact *datura.Artifact) {
	if tree == nil || artifact == nil {
		return
	}

	wire := artifact.Marshal()

	if len(wire) == 0 {
		return
	}

	tree.Insert(artifact.Prefix(), wire)
}

/*
InsertMeasurement indexes a publishable classifier measurement in the shared tree.
*/
func InsertMeasurement(tree *dmt.Tree, artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	if datura.Peek[int](artifact, "classifier.category") <= 0 {
		return
	}

	if datura.Peek[float64](artifact, "classifier.confidence") <= 0 {
		return
	}

	artifact.WithRole("measurement")
	InsertTreeArtifact(tree, artifact)
}
