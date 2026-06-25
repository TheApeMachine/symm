package market

import (
	"iter"

	"github.com/theapemachine/datura"
)

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(*datura.Artifact, *CrossSection) iter.Seq[*datura.Artifact]
	IngestRoles() []string
	Close() error
}
