package types

/*
The fluid view's data channels. ManifoldChannel carries the resident sensorium
State and Reading as one ManifoldFrame per Step; DiagnosticsChannel carries the
replaceable diagnostics snapshot. Both ride the same WebRTC transport.
*/
const (
	ManifoldChannel = "manifold"

	// ResonanceChannel carries the predictive-coder resonance artifact (layers,
	// latent, frame/dynamics, forecast) that previously rode the websocket every
	// ticker envelope.
	ResonanceChannel = "resonance"

	// DiagnosticsChannel carries the replaceable diagnostics snapshot. It rides
	// the same WebRTC transport as the manifold channel so the diagram can be
	// opened without the orchestrating websocket bus in front of the data.
	DiagnosticsChannel = "diagnostics"
)
