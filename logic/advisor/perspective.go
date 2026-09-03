package advisor

import (
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/vector"
	"github.com/theapemachine/symm/types"
)

/* Issuer owns the identity and lifecycle sequence of one Advisor's claims. */
type Issuer struct {
	name     nmtypes.Symbol
	features []*Feature
	groups   []vector.Group
	clock    nmtypes.Symbol
	rounds   map[string]uint64
	sequence uint64
}

func newIssuer(
	name string,
	features []*Feature,
	groups []vector.Group,
	clock nmtypes.Symbol,
) *Issuer {
	return &Issuer{
		name:     nmtypes.MustIntern(name),
		features: slices.Clone(features),
		groups:   groups,
		clock:    clock,
		rounds:   make(map[string]uint64),
	}
}

/* Name returns the stable declared Advisor identity. */
func (issuer *Issuer) Name() string {
	name, _ := nmtypes.SymbolName(issuer.name)

	return name
}

func envelopeAt(envelope *types.Envelope) time.Time {
	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Timestamp
	case types.EnvelopeTrade:
		return envelope.TradeData.Timestamp
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Timestamp
	case types.EnvelopeFuturesTicker:
		return envelope.FuturesTickerData.Timestamp
	case types.EnvelopeFuturesTrade:
		return envelope.FuturesTradeData.Timestamp
	default:
		return time.Time{}
	}
}

/* Issue emits one falsifiable round when the distribution has a unique lean. */
func (issuer *Issuer) Issue(
	envelope *types.Envelope,
	frame nmtypes.Frame,
	coordinate uint64,
) error {
	distribution := vector.Unpack(frame, issuer.groups)

	if !distribution.Ready || distribution.Sharpness <= 0 {
		return nil
	}

	if distribution.WinnerIndex < 0 || distribution.WinnerIndex >= len(issuer.features) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[advisor] winning class is outside the declared Feature set",
			nil,
		))
	}

	feature := issuer.features[distribution.WinnerIndex]

	if len(feature.Class.Predictions) == 0 {
		return nil
	}

	if feature.Class.Within == 0 || coordinate > ^uint64(0)-feature.Class.Within {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[advisor] winning class requires a valid prediction horizon",
			nil,
		))
	}

	classes := make([]types.PerspectiveClass, len(issuer.groups))

	for index, group := range issuer.groups {
		classes[index] = types.PerspectiveClass{
			State:       types.PerspectiveState(group.Label),
			Probability: distribution.Probabilities[group.Label],
		}
	}

	predictions := issuer.predictions()
	symbol := envelopeSymbol(envelope)
	issuer.rounds[symbol]++
	issuer.sequence++
	envelope.Perspectives = append(envelope.Perspectives, &types.Perspective{
		Symbol:      symbol,
		Advisor:     issuer.name,
		Question:    types.PerspectiveQuestion(issuer.Name()),
		IssuedAt:    envelopeAt(envelope),
		Sequence:    issuer.sequence,
		Round:       issuer.rounds[symbol],
		Classes:     classes,
		Predictions: predictions,
		Lease: types.PerspectiveLease{
			Clock: issuer.clock,
			From:  coordinate,
			Until: coordinate + feature.Class.Within,
		},
		Lifecycle: types.PerspectiveIssued,
	})

	return nil
}

func (issuer *Issuer) predictions() []types.PerspectivePrediction {
	count := 0

	for _, feature := range issuer.features {
		count += 2 * len(feature.Class.Predictions)
	}

	predictions := make([]types.PerspectivePrediction, 0, count)

	for _, feature := range issuer.features {
		for _, prediction := range feature.Class.Predictions {
			class := types.PerspectiveState(feature.Class.Label)
			predictions = append(predictions,
				types.PerspectivePrediction{
					Class:  class,
					Event:  types.PerspectiveEvent(prediction.Support.Label),
					Effect: types.PredictionSupports,
				},
				types.PerspectivePrediction{
					Class:  class,
					Event:  types.PerspectiveEvent(prediction.Contradict.Label),
					Effect: types.PredictionFalsifies,
				},
			)
		}
	}

	return predictions
}
