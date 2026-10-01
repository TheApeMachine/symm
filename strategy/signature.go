package strategy

import "bytes"

/*
appendLitFrame extends a null-separated LitRegions signature only on change.
Identical consecutive frames are a temporal hold, not a new precursor step
(TRAINING.md: temporal signature from priors). AAA → A, ABBA → ABA.
*/
func appendLitFrame(signature, token []byte) []byte {
	if len(token) == 0 {
		return signature
	}

	if bytes.Equal(lastLitFrame(signature), token) {
		return signature
	}

	signature = append(signature, token...)
	return append(signature, 0)
}

/*
lastLitFrame returns the trailing LitRegions frame (bytes after the last NUL),
or nil when the signature is empty.
*/
func lastLitFrame(signature []byte) []byte {
	if len(signature) == 0 {
		return nil
	}

	end := len(signature)

	if signature[end-1] == 0 {
		end--
	}

	if end == 0 {
		return nil
	}

	start := end

	for start > 0 && signature[start-1] != 0 {
		start--
	}

	return signature[start:end]
}
