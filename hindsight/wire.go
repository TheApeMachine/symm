package hindsight

import (
	"encoding/json"
)

/*
MarshalIdentity encodes a CaptureIdentity as a stable string for persistence.
The representation is backend-agnostic: it is the identity's own fields, never a
storage row id, so it survives a storage engine swap unchanged (§50).
*/
func MarshalIdentity(identity CaptureIdentity) (string, error) {
	encoded, err := json.Marshal(identity)

	if err != nil {
		return "", errnieEncoding("hindsight: marshal capture identity", err)
	}

	return string(encoded), nil
}

/*
UnmarshalIdentity decodes a persisted CaptureIdentity string back into the
identity it represents. A blank or malformed payload is a loud failure, never a
silent zero identity (§43, §48).
*/
func UnmarshalIdentity(encoded string) (CaptureIdentity, error) {
	var identity CaptureIdentity

	if err := json.Unmarshal([]byte(encoded), &identity); err != nil {
		return CaptureIdentity{}, errnieEncoding("hindsight: unmarshal capture identity", err)
	}

	if !identity.Valid() {
		return CaptureIdentity{}, errnieEncoding("hindsight: persisted capture identity is invalid", nil)
	}

	return identity, nil
}

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
UnmarshalEnvelopeRef decodes a persisted EnvelopeRef string back.
*/
func UnmarshalEnvelopeRef(encoded string) (EnvelopeRef, error) {
	var ref EnvelopeRef

	if err := json.Unmarshal([]byte(encoded), &ref); err != nil {
		return EnvelopeRef{}, errnieEncoding("hindsight: unmarshal envelope ref", err)
	}

	if !ref.Origin.Valid() {
		return EnvelopeRef{}, errnieEncoding("hindsight: persisted envelope ref has an invalid origin", nil)
	}

	return ref, nil
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
