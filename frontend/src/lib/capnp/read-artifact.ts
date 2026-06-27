import * as capnp from "capnp-ts";

import { Artifact, type ArtifactRoot } from "#/lib/capnp/artifact";

const parseJSONRecord = (text: string): Record<string, unknown> => {
	const trimmed = text.trim();

	if (trimmed === "") {
		return {};
	}

	return JSON.parse(trimmed) as Record<string, unknown>;
};

const readAttributesJSON = (
	artifact: ArtifactRoot,
): Record<string, unknown> => {
	if (!artifact.hasAttributes()) {
		return {};
	}

	const bytes = artifact.getAttributes().toUint8Array();
	const text = new TextDecoder().decode(bytes);

	return parseJSONRecord(text);
};

type PayloadArtifactRoot = ArtifactRoot & {
	hasPayload?: () => boolean;
	getPayload?: () => capnp.Data;
	hasPublicKey?: () => boolean;
	getPublicKey?: () => capnp.Data;
};

const hasDataField = (
	artifact: PayloadArtifactRoot,
	upper: "Payload" | "PublicKey",
): boolean => {
	const lowerName = `has${upper}` as keyof PayloadArtifactRoot;
	const fn = artifact[lowerName] as unknown;
	if (typeof fn === "function") {
		return Boolean((fn as () => boolean).call(artifact));
	}

	return false;
};

const dataFieldBytes = (
	artifact: PayloadArtifactRoot,
	upper: "Payload" | "PublicKey",
): Uint8Array => {
	const directName = `get${upper}` as keyof PayloadArtifactRoot;
	const fn = artifact[directName] as unknown;
	if (typeof fn === "function") {
		return (fn as () => capnp.Data).call(artifact).toUint8Array();
	}

	return new Uint8Array();
};

const dataLength = (data: capnp.Data | undefined): number => {
	if (!data) {
		return 0;
	}

	return data.toUint8Array().length;
};

const hasNonEmptyEncryptedKey = (artifact: ArtifactRoot): boolean => {
	if (!artifact.hasEncryptedKey()) {
		return false;
	}

	return dataLength(artifact.getEncryptedKey()) > 0;
};

const hasSealedPayload = (artifact: PayloadArtifactRoot): boolean => {
	if (!hasDataField(artifact, "PublicKey")) {
		return false;
	}

	return dataFieldBytes(artifact, "PublicKey").length > 0;
};

const readPayloadJSON = (
	artifact: ArtifactRoot,
): Record<string, unknown> => {
	const payloadArtifact = artifact as PayloadArtifactRoot;
	if (!hasDataField(payloadArtifact, "Payload")) {
		return {};
	}

	if (hasSealedPayload(payloadArtifact) || hasNonEmptyEncryptedKey(artifact)) {
		return {};
	}

	const payload = dataFieldBytes(payloadArtifact, "Payload");
	const payloadText = new TextDecoder().decode(payload);

	return parseJSONRecord(payloadText);
};

const timestampToBigInt = (value: unknown): bigint => {
	if (typeof value === "bigint") {
		return value;
	}

	if (typeof value === "number" && Number.isFinite(value)) {
		return BigInt(Math.trunc(value));
	}

	if (typeof value === "string" && value.trim() !== "") {
		return BigInt(value);
	}

	if (
		typeof value === "object" &&
		value !== null &&
		"toString" in value &&
		typeof value.toString === "function"
	) {
		return BigInt(value.toString());
	}

	return 0n;
};

const readTimestampFields = (artifact: ArtifactRoot): Record<string, unknown> => {
	const unixNano = timestampToBigInt(artifact.getTimestamp());

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

	try {
		const message = new capnp.Message(wire, true);
		const artifact = message.getRoot(Artifact);
		const attributesJSON = readAttributesJSON(artifact);
		const payloadJSON = readPayloadJSON(artifact);

		return {
			...readTimestampFields(artifact),
			...attributesJSON,
			...payloadJSON,
			role: artifact.getRole(),
			scope: artifact.getScope(),
			origin: artifact.getOrigin(),
			destination: artifact.getDestination(),
		};
	} catch {
		return null;
	}
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

	try {
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
	} catch {
		return null;
	}
};
