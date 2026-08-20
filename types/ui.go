package types

import wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

/*
UIFrame is one immutable schema object awaiting transport batching. Producers
publish this representation directly so the websocket hub performs the only
FlatBuffers encoding and never copies separately encoded child frames.
*/
type UIFrame = wire.FrameT
