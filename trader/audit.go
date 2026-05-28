package trader

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/errnie"
)

/*
audit emits one JSON line for offline agent analysis.
*/
func audit(event string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}

	fields["event"] = event
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	payload, err := json.Marshal(fields)

	if err != nil {
		errnie.Error(err)
		return
	}

	errnie.Info(string(payload))
}
