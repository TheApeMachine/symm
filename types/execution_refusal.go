package types

/*
	ExecutionRefusal is an economic or authority precondition rejected before

submission. It is not evidence that venue execution failed.
*/
type ExecutionRefusal struct {
	State  string
	Detail string
}

/* Error preserves the refused precondition at the broker boundary. */
func (refusal *ExecutionRefusal) Error() string { return refusal.State + ": " + refusal.Detail }
