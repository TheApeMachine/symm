package equation

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEvidenceShare selects one member after normalization. The index is a
// configured source. An absent index reports a shape error, while zero total
// mass remains undefined; neither becomes an apparently observed zero share.
func NewEvidenceShare(index core.Primitive) core.Primitive {
	return transport.NewPipe(NewNormalize(), transport.NewCollect[float64](), collection.NewAt[float64](index))
}
