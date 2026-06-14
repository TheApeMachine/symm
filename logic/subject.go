package logic

import (
	"cmp"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/user"
)

type SubjectType string

const (
	SubjectNone       SubjectType = ""
	SubjectCategory   SubjectType = "category"
	SubjectRegime     SubjectType = "regime"
	SubjectPosition   SubjectType = "position"
	SubjectHolding    SubjectType = "holding"
	SubjectPrice      SubjectType = "price"
	SubjectVolume     SubjectType = "volume"
	SubjectSpread     SubjectType = "spread"
	SubjectElapsed    SubjectType = "elapsed"
	SubjectStrength   SubjectType = "strength"
	SubjectConfidence SubjectType = "confidence"
	SubjectSurprise   SubjectType = "surprise"
	SubjectEigenmode  SubjectType = "eigenmode"
	SubjectModeShare  SubjectType = "mode_share"
)

func (subjectType SubjectType) Evaluate(
	measurement *Measurement,
	holdings *user.Balances,
	right any,
) (int, error) {
	switch subjectType {
	case SubjectCategory:
		return cmp.Compare(measurement.Category, right.(CategoryType)), nil
	case SubjectRegime:
		return cmp.Compare(measurement.Regime, right.(RegimeType)), nil
	case SubjectPosition:
		return cmp.Compare(measurement.Position, right.(PositionType)), nil
	case SubjectHolding:
		for _, wallet := range holdings.Asset {
			if wallet.Asset == measurement.Symbol {
				return 1, nil
			}
		}

		return -1, nil
	case SubjectPrice:
		return cmp.Compare(measurement.Price, right.(float64)), nil
	case SubjectVolume:
		return cmp.Compare(measurement.Volume, right.(float64)), nil
	case SubjectSpread:
		return cmp.Compare(measurement.Spread, right.(float64)), nil
	case SubjectElapsed:
		return cmp.Compare(measurement.Elapsed, right.(float64)), nil
	case SubjectStrength:
		return cmp.Compare(measurement.Strength, right.(float64)), nil
	case SubjectConfidence:
		return cmp.Compare(measurement.Confidence, right.(float64)), nil
	case SubjectSurprise:
		return cmp.Compare(measurement.Surprise, right.(float64)), nil
	default:
		return 0, errnie.Error(errnie.Err(
			errnie.IO,
			"logic: invalid subject type",
			nil,
		))
	}
}
