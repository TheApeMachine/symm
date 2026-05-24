package engine

/*
MarketPerspective groups signals by the market angle they measure.
The trader combines perspectives selectively, not all sources equally.
*/
type MarketPerspective string

const (
	PerspectiveMicrostructure MarketPerspective = "microstructure"
	PerspectiveFlow           MarketPerspective = "flow"
	PerspectiveCrossAsset     MarketPerspective = "cross_asset"
	PerspectiveSentiment      MarketPerspective = "sentiment"
)

/*
SourcePerspective maps each signal source to its primary market angle.
*/
func SourcePerspective(source string) MarketPerspective {
	perspective, ok := sourcePerspectives[source]

	if !ok {
		return PerspectiveMicrostructure
	}

	return perspective
}

var sourcePerspectives = map[string]MarketPerspective{
	"pumpdump":  PerspectiveMicrostructure,
	"hawkes":    PerspectiveMicrostructure,
	"depthflow": PerspectiveMicrostructure,
	"fluid":     PerspectiveFlow,
	"leadlag":   PerspectiveCrossAsset,
	"basis":     PerspectiveCrossAsset,
	"sentiment": PerspectiveSentiment,
	"causal":    PerspectiveSentiment,
}
