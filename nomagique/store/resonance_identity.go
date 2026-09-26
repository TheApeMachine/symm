package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/* identify validates ordered coordinate meaning before any retained state advances. */
func (server *ResonanceServer) identify(labels capnp.TextList, width int) error {
	if labels.Len() != width {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance: feature identities must name every coordinate", nil))
	}
	candidate := make([]string, width)
	seen := make(map[string]bool, width)
	for index := range width {
		identity, err := labels.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		if identity == "" || seen[identity] {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance: feature identities must be nonempty and unique", nil))
		}
		if len(server.identities) > 0 && (len(server.identities) != width || server.identities[index] != identity) {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance: feature vocabulary differs from retained model", nil))
		}
		seen[identity], candidate[index] = true, identity
	}
	if len(server.identities) == 0 {
		server.identities = candidate
	}
	return nil
}
