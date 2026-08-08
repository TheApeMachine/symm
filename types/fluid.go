package types

/*
The fluid view's two data channels. A frame belongs to one of them, and the
manifold solver knows which the moment it builds one.
*/
const (
	FluidFieldsChannel    = "fluid-fields"
	FluidParticlesChannel = "fluid-particles"
)

/*
FluidFrame is one manifold publication addressed to the data channel that
carries it.

The channel used to be recovered on the consuming end by testing the payload
for a `{"fields":` or `{"particles":` prefix, which re-derived something the
producer already knew and made the routing depend on how the payload happened
to be encoded. Naming the destination keeps that knowledge where it originates
and leaves Payload free to become an opaque binary encoding.
*/
type FluidFrame struct {
	Channel string
	Payload []byte
}
