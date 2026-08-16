package nomagique

/*
FrameFromNamed converts a string-keyed boundary map into a Frame. It interns
names and may allocate; use package-level symbols and Put in reducer hot paths.
*/
func FrameFromNamed(values map[string]float64) (Frame, error) {
	frame := Frame{}

	for name, value := range values {
		symbol, err := Intern(name)

		if err != nil {
			return Frame{}, err
		}

		frame.Put(symbol, value)
	}

	return frame, nil
}

/*
Named converts populated slots into a string-keyed boundary map. It allocates
and is intended for diagnostics, serialization, and external APIs.
*/
func (frame Frame) Named() map[string]float64 {
	values := make(map[string]float64, frame.Count())

	for symbol, value := range frame.All() {
		name, found := SymbolName(symbol)

		if found {
			values[name] = value
		}
	}

	return values
}
