package fluid

/*
FieldSink receives a field snapshot the moment it is computed.
*/
type FieldSink func(snapshot FieldSnapshot)
