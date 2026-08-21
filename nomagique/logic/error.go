package logic

type logicError string

func (err logicError) Error() string {
	return "logic: " + string(err) + " must emit condition"
}

func conditionError(source string) error {
	return logicError(source)
}
