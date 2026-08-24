package algo

import (
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
LeadLag composes the asynchronous cross-lag search with normalized evidence
projection. It owns no path or cohort state; the caller supplies committed
anchor and follower Path Frames.
*/
func LeadLag() types.Primitive {
	return types.Pipe(
		correlation.CrossLag,
		correlation.LeadLagScores,
	)
}
