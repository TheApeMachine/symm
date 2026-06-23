import * as capnp from "capnp-ts";

import { Artifact, type ArtifactRoot } from "#/lib/capnp/artifact";

const AES_GCM_NONCE_BYTES = 12;
const AES_GCM_KEY_BYTES = 32;

const parseJSONRecord = (text: string): Record<string, unknown> => {
	const trimmed = text.trim();

	if (trimmed === "") {
		return {};
	}

	return JSON.parse(trimmed) as Record<string, unknown>;
};

const readAttributesJSON = (artifact: ArtifactRoot): Record<string, unknown> => {
	if (!artifact.hasAttributes()) {
		return {};
	}

	const bytes = artifact.getAttributes().toUint8Array();
	const text = new TextDecoder().decode(bytes);

	return parseJSONRecord(text);
};

const decryptPayloadJSON = async (
	artifact: ArtifactRoot,
): Promise<Record<string, unknown>> => {
	if (!artifact.hasEncryptedPayload() || !artifact.hasEncryptedKey()) {
		return {};
	}

	const encryptedKey = new Uint8Array(
		artifact.getEncryptedKey().toUint8Array(),
	);
	const encryptedPayload = new Uint8Array(
		artifact.getEncryptedPayload().toUint8Array(),
	);

	if (
		encryptedKey.length !== AES_GCM_KEY_BYTES ||
		encryptedPayload.length <= AES_GCM_NONCE_BYTES
	) {
		return {};
	}

	const cryptoKey = await crypto.subtle.importKey(
		"raw",
		encryptedKey,
		"AES-GCM",
		false,
		["decrypt"],
	);
	const plaintext = await crypto.subtle.decrypt(
		{
			name: "AES-GCM",
			iv: encryptedPayload.slice(0, AES_GCM_NONCE_BYTES),
		},
		cryptoKey,
		encryptedPayload.slice(AES_GCM_NONCE_BYTES),
	);
	const payloadText = new TextDecoder().decode(plaintext);

	return parseJSONRecord(payloadText);
};

const readTimestampFields = (
	artifact: ArtifactRoot,
): Record<string, unknown> => {
	const unixNano = artifact.getTimestamp();

	if (unixNano <= 0n) {
		return {};
	}

	return {
		timestamp_unix_nano: unixNano.toString(),
		observed_at: Number(unixNano / 1_000_000n),
	};
};

/*
decodePackedArtifactWire decodes a packed capnp artifact wire frame into dashboard JSON.
Attributes and decrypted payload are merged with capnp identity fields.
*/
export const decodePackedArtifactWire = async (
	wire: ArrayBuffer,
): Promise<Record<string, unknown> | null> => {
	if (wire.byteLength === 0) {
		return null;
	}

	const message = new capnp.Message(wire, true);
	const artifact = message.getRoot(Artifact);
	const attributesJSON = readAttributesJSON(artifact);
	const payloadJSON = await decryptPayloadJSON(artifact);

	return {
		...readTimestampFields(artifact),
		...attributesJSON,
		...payloadJSON,
		role: artifact.getRole(),
		scope: artifact.getScope(),
		origin: artifact.getOrigin(),
		destination: artifact.getDestination(),
	};
};

/*
artifactFrameFromWire decodes a packed capnp artifact wire frame into dashboard JSON.
*/
export const artifactFrameFromWire = (
	wire: ArrayBuffer,
): Record<string, unknown> | null => {
	if (wire.byteLength === 0) {
		return null;
	}

	const message = new capnp.Message(wire, true);
	const artifact = message.getRoot(Artifact);

	return {
		...readTimestampFields(artifact),
		...readAttributesJSON(artifact),
		origin: artifact.getOrigin(),
		scope: artifact.getScope(),
		role: artifact.getRole(),
		destination: artifact.getDestination(),
	};
};
