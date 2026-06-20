import * as capnp from "capnp-ts";

import { Artifact, type ArtifactRoot } from "#/lib/capnp/artifact";

/*
artifactFrameFromWire decodes a capnp artifact wire frame into dashboard JSON.
*/
export const artifactFrameFromWire = (
	wire: ArrayBuffer,
): Record<string, unknown> | null => {
	if (wire.byteLength === 0) {
		return null;
	}

	const message = new capnp.Message(wire, false);
	const artifact = message.getRoot(Artifact);

	let attributesJSON: Record<string, unknown> = {};

	if (artifact.hasAttributes()) {
		const raw = artifact.getAttributes();
		const bytes = raw.toUint8Array();
		const text = new TextDecoder().decode(bytes).trim();

		if (text !== "") {
			attributesJSON = JSON.parse(text) as Record<string, unknown>;
		}
	}

	return {
		...attributesJSON,
		origin: artifact.getOrigin(),
		scope: artifact.getScope(),
		role: artifact.getRole(),
		destination: artifact.getDestination(),
	};
};
