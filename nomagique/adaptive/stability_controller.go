package adaptive

/*
StabilityController provides online directional feedback into Governor nodes
using an adaptive window policy (e.g. KISH, ADWIN, STABILITY_GOV).
*/
type StabilityController struct {
	Type WindowType

	window Window
}

func (controller *StabilityController) Step(value float64) int {
	controller.window.Type = controller.Type

	return controller.window.Step(value)
}

func (controller *StabilityController) Capacity() int {
	return controller.window.Capacity()
}
