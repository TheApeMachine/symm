package replay

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

// Audit counts persisted input acceptance and ingress manifests from the
// isolated replay run. Completed is read separately from dashboard telemetry.
type Audit struct {
	Directory string
	Captures  uint64
	Manifests int64
	seen      map[string]bool
}

func (audit *Audit) Read() error {
	if audit.seen == nil {
		audit.seen = make(map[string]bool)
	}

	for _, family := range []string{"captures", "manifests"} {
		paths, err := filepath.Glob(filepath.Join(audit.Directory, "replay", family, "*", "*.jsonl"))

		if err != nil {
			return errnie.Error(err)
		}

		for _, path := range paths {
			if audit.seen[path] {
				continue
			}
			payload, err := os.ReadFile(path)

			if err != nil {
				return errnie.Error(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(payload))

			for {
				var record json.RawMessage
				err := decoder.Decode(&record)

				if err == io.EOF {
					break
				}

				if err != nil {
					return errnie.Error(err)
				}

				if family == "manifests" {
					var manifest hindsight.EnvelopeManifest
					if err := json.Unmarshal(record, &manifest); err != nil {
						return errnie.Error(err)
					}

					if manifest.Envelope.Origin.Sequence == 0 {
						return errnie.Error(errnie.Err(errnie.Validation, "replay: manifest has no capture identity", nil))
					}
					audit.Manifests++
					continue
				}
				var frame hindsight.RawFrame

				if err := json.Unmarshal(record, &frame); err != nil {
					return errnie.Error(err)
				}

				if frame.Kind != "l3_touch" {
					audit.Captures++
				}
			}
			audit.seen[path] = true
		}
	}
	return nil
}
