package logic

/*
MatchAttribution identifies the source/category that triggered a playbook branch.
*/
type MatchAttribution struct {
	Source   SourceType
	Category CategoryType
}

/*
AttributionFromConditionGroup prefers exit-oriented categories over benign window tail.
*/
func AttributionFromConditionGroup(
	conditionGroup *ConditionGroup,
	measurements []Measurement,
	holdings *Holdings,
) (MatchAttribution, bool) {
	if conditionGroup == nil {
		return MatchAttribution{}, false
	}

	bestTier := ExitTier(0)
	best := MatchAttribution{}

	for _, condition := range conditionGroup.Conditions {
		if condition.Type != ConditionIsTrue {
			continue
		}

		if condition.Left.Subject.Type != SubjectCategory {
			continue
		}

		if condition.Left.Subject.Category == nil {
			continue
		}

		targetCategory := condition.Left.Subject.Category.Type
		sourceFilter := condition.Left.Subject.Source

		for _, measurement := range measurements {
			if sourceFilter != SourceNone && measurement.Source != sourceFilter {
				continue
			}

			if measurement.Category != targetCategory {
				continue
			}

			tier := ExitTierForCategory(measurement.Category)

			if best.Category == CategoryTypeNone || tierPriority(tier) > tierPriority(bestTier) {
				best = MatchAttribution{
					Source:   measurement.Source,
					Category: measurement.Category,
				}
				bestTier = tier
			}
		}
	}

	if best.Category == CategoryTypeNone {
		return MatchAttribution{}, false
	}

	return best, true
}

func tierPriority(tier ExitTier) int {
	switch tier {
	case ExitTierThesisInvalidation:
		return 3
	case ExitTierRiskDeterioration:
		return 2
	case ExitTierProfitExhaustion:
		return 1
	default:
		return 0
	}
}
