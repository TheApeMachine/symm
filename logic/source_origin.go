package logic

/*
SourceFromSignalOrigin maps dashboard signal registry names to spectrum source ids.
*/
func SourceFromSignalOrigin(origin string) SourceType {
	switch origin {
	case "exhaust":
		return SourceExhaustion
	default:
		return SourceType(origin)
	}
}
