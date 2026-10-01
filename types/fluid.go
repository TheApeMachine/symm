package types

/*
The fluid / diagnostics view channel names. Manifold frames ride the hub
WebSocket (ui.UITee → /ws) as FrameManifoldFrame. Resonance artifacts ride the
same websocket as MeasurementsFrame rows with Source "resonance". Diagnostics
topology, when published, uses the same socket rather than a separate transport.
*/
const (
	ManifoldChannel = "manifold"

	// ResonanceChannel names the predictive-coder resonance artifact (layers,
	// latent, frame/dynamics, forecast) published as measurement rows.
	ResonanceChannel = "resonance"

	// DiagnosticsChannel names the replaceable diagnostics snapshot family.
	DiagnosticsChannel = "diagnostics"

	// CognitionChannel carries the real-time cognitive state (sensory prefix tree
	// branches, lookahead beams, regime classes, entropy, and REM sleep replays).
	CognitionChannel = "cognition"
)
