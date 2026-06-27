package broker

import (
	"time"

	"github.com/theapemachine/datura"
)

func (desk *Desk) publishDiagnostic(
	severity, code, message string,
	context datura.Map[any],
) {
	if desk == nil {
		return
	}
	if context == nil {
		context = datura.Map[any]{}
	}

	artifact := datura.Acquire("broker", datura.APPJSON).
		WithDestination("ui").
		WithRole("diagnostic").
		WithScope(code).
		WithPayload(datura.Map[any]{
			"channel":     "diagnostic",
			"severity":    severity,
			"component":   "broker.desk",
			"code":        code,
			"message":     message,
			"context":     context,
			"observed_at": time.Now().UTC().Format(time.RFC3339Nano),
		}.Marshal())
	artifact.SetTimestamp(time.Now().UnixNano())

	if desk.tree != nil {
		desk.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
	}
	if desk.pool != nil {
		_ = desk.pool.CreateBroadcastGroup("ui").Send(artifact)
	}
}
