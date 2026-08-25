package equation

import (
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
CategoryClassify is a named composition of atomic probability primitives:

	Argmax (select the strongest category)
	-> EvidenceShare (symmetric one-pseudocount confidence)
	-> ShannonAmbiguity (normalized distribution entropy)

It is an equation: zero implementation code, a pure wiring diagram over the
generic sample slots. It reads the per-category strength vector, writes the
winning index, its confidence, and the distribution ambiguity, and leaves the
identity/role/meaning boundary to the domain adapter that lifts strengths into a
Frame.
*/
func CategoryClassify() types.Primitive {
	return types.Pipe(
		probability.Argmax(),
		probability.EvidenceShare(),
		probability.ShannonAmbiguity(),
	)
}
