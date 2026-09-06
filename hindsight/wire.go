package hindsight

import (
	"encoding/json"
)

/*
MarshalEnvelopeRef encodes an EnvelopeRef as a stable string for persistence.
*/
func MarshalEnvelopeRef(ref EnvelopeRef) (string, error) {
	encoded, err := json.Marshal(ref)

	if err != nil {
		return "", errnieEncoding("hindsight: marshal envelope ref", err)
	}

	return string(encoded), nil
}

/*
MarshalManifest encodes an EnvelopeManifest as a stable string for persistence.
*/
func MarshalManifest(manifest EnvelopeManifest) (string, error) {
	encoded, err := json.Marshal(manifest)

	if err != nil {
		return "", errnieEncoding("hindsight: marshal envelope manifest", err)
	}

	return string(encoded), nil
}

/*
MarshalWitness encodes an ArtifactWitness as a stable string for persistence.
*/
func MarshalWitness(witness ArtifactWitness) (string, error) {
	encoded, err := json.Marshal(witness)

	if err != nil {
		return "", errnieEncoding("hindsight: marshal artifact witness", err)
	}

	return string(encoded), nil
}

/*
errnieEncoding builds the package's canonical encoding error.
*/
func errnieEncoding(message string, cause error) error {
	return encodingError{message: message, cause: cause}
}

/*
encodingError is a persistence-encoding failure.
*/
type encodingError struct {
	message string
	cause   error
}

func (err encodingError) Error() string {
	if err.cause == nil {
		return err.message
	}

	return err.message + ": " + err.cause.Error()
}

func (err encodingError) Unwrap() error {
	return err.cause
}
