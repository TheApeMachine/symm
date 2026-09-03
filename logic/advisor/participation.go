package advisor

const (
	ParticipationName              = "participation"
	participationClock             = "pumpdump/completed_volume_bar_ordinal"
	participationPredictionHorizon = uint64(1)
)

/*
Participation classifies market-wide cohort breadth and leadership dynamics.
*/
type Participation struct {
	Features []*Feature
}

/*
NewParticipation constructs the Participation advisor with competing features
and falsifiable predictions covering broad lift, local leaders, followers, and
isolated idiosyncratic moves.
*/
func NewParticipation() *Participation {
	broadLiftKeys := []string{
		"sentiment/advance_fraction",
		"sentiment/directional_participation",
		"sentiment/breadth",
		"sentiment/same_direction_peer_fraction",
	}

	localLeaderKeys := []string{
		"correlation/relative_return_energy",
		"sentiment/largest_move_share",
		"correlation/cohort_signed_correlation",
	}

	followerMoveKeys := []string{
		"leadlag/best_lag_seconds",
		"leadlag/contemporaneous_correlation",
		"leadlag/lag_fraction",
	}

	isolatedMoveKeys := []string{
		"correlation/relative_return_energy",
		"sentiment/largest_move_excess",
		"correlation/cohort_correlation_dispersion",
	}

	return &Participation{
		Features: []*Feature{
			NewFeature(
				participationClock,
				broadLiftKeys,
				&Class{
					Label:  "BroadLift",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"sentiment/advance_fraction",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				localLeaderKeys,
				&Class{
					Label:  "LocalLeader",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"correlation/relative_return_energy",
						INCREASE,
						DECREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				followerMoveKeys,
				&Class{
					Label:  "FollowerMove",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"leadlag/best_lag_seconds",
						DECREASE,
						INCREASE,
					)},
				},
			),
			NewFeature(
				participationClock,
				isolatedMoveKeys,
				&Class{
					Label:  "IsolatedMove",
					Within: participationPredictionHorizon,
					Predictions: []*Prediction{NewMetricPrediction(
						"correlation/relative_return_energy",
						EXPAND,
						DISSOLVE,
					)},
				},
			),
		},
	}
}
