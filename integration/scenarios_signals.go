package integration

/*
signalScenarios runs one strict validation scenario per signal category.
Each scenario replays a dedicated synthetic fixture and requires an exact category
on the probe symbol — not "any of" the source's labels.
*/
func signalScenarios() []Scenario {
	return signalValidationScenarios()
}
