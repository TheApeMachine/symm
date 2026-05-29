package perspectives

type PumpPerspective struct {
	Tree *Tree
}

/*
NewPumpPerspective creates a new PumpPerspective from a slice of measurements.
The Pump Perspective break down to the following decisions:

- The flash pump and dump
 1. We see some signal of coiled compression (slow breakout or pre-spike precursor)
 2. Followed by a signal of vertical ignition (spike up)
 3. We enter the position with a stop loss that we ratchet up with the spike
 4. We let the dump drop right into our stop loss

- The slow pump
 1. We see some signal of coiled compression (slow breakout or pre-spike precursor)
 2. We enter the position with a stop loss that we ratchet up with the spike
 3. We let the dump drop right into our stop loss

- Spoof onto them as they spoof onto us
 1. We see a crystal clear signal of spoofing happening (fake bid/ask volume)
 2. We enter the position early
 3. We dedicate a high-priority process to monitor the asset pair
 4. The moment we see the signal spike we predict the best moment to switch our long to a short positoin
*/
func NewPumpPerspective() *PumpPerspective {
	return &PumpPerspective{
		Tree: &Tree{
			Branch: &map[CategoryType]*Tree{
				CategoryCoiledCompression: {
					Branch: &map[CategoryType]*Tree{
						CategoryVerticalIgnition: {
							Action: &Action{
								ActionType: ActionStopLoss,
								Value:      0.0,
							},
						},
						CategoryOrganicTrend: {
							Observation: &Observation{
								ObservationType: ObservationHasContinued,
								Branch: &map[ObservationType]*Tree{
									ObservationHolding: {
										Action: &Action{
											ActionType: ActionTakeProfit,
											Condition:  ConditionIsGreaterThanOrEqual,
											Unit:       UnitPercentage,
											Value:      150,
										},
									},
									ObservationNotHolding: {
										Action: &Action{
											ActionType: ActionStopLoss,
											Value:      0.0,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
