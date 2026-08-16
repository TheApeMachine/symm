package nomagique

/*
Number evaluates one primitive transition. It remains as a small migration aid
for call sites that used nomagique.Number as their composition entry point.
*/
func Number(
	primitive Primitive,
	state Frame,
	input Frame,
) (Frame, Frame, error) {
	return Step(primitive, state, input)
}
