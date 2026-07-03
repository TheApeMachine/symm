import * as capnp from "capnp-ts";

import { Artifact, type ArtifactRoot } from "#/lib/capnp/artifact";

type PayloadArtifactRoot = ArtifactRoot & {
	hasAttributes?: () => boolean;
	getAttributes?: () => capnp.Data;
	hasPayload?: () => boolean;
	getPayload?: () => capnp.Data;
};

const dataFieldBytes = (
	artifact: PayloadArtifactRoot,
	upper: "Attributes" | "Payload",
): Uint8Array => {
	const hasName = `has${upper}` as keyof PayloadArtifactRoot;
	const has = artifact[hasName] as unknown;
	if (typeof has === "function" && !(has as () => boolean).call(artifact)) {
		return new Uint8Array();
	}

	const directName = `get${upper}` as keyof PayloadArtifactRoot;
	const fn = artifact[directName] as unknown;
	if (typeof fn === "function") {
		return (fn as () => capnp.Data).call(artifact).toUint8Array();
	}

	return new Uint8Array();
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

const readTimestampFields = (
	artifact: ArtifactRoot,
): Record<string, unknown> => {
	const unixNano = timestampToBigInt(artifact.getTimestamp());

	if (unixNano <= 0n) {
		return {};
	}

	return {
		timestamp_unix_nano: unixNano.toString(),
		observed_at: Number(unixNano / 1_000_000n),
	};
};

const jsonFromBytes = (bytes: Uint8Array): Record<string, unknown> | null => {
	if (bytes.length === 0) {
		return {};
	}

	const text = new TextDecoder().decode(bytes).trim();
	if (text === "") {
		return {};
	}

	return JSON.parse(text) as Record<string, unknown>;
};

export const artifactFrameFromWire = (
	wire: ArrayBuffer,
): Record<string, unknown> | null => {
	if (wire.byteLength === 0) {
		return null;
	}

	try {
		const message = new capnp.Message(wire, true);
		const artifact = message.getRoot(Artifact);
		const payloadArtifact = artifact as PayloadArtifactRoot;
		const attributesJSON = jsonFromBytes(
			dataFieldBytes(payloadArtifact, "Attributes"),
		);
		const payloadJSON = jsonFromBytes(dataFieldBytes(payloadArtifact, "Payload"));

		if (attributesJSON === null || payloadJSON === null) {
			return null;
		}

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
decodePackedArtifactWire decodes a packed capnp artifact wire frame into dashboard JSON.
Attributes and plaintext payload are merged with capnp identity fields.
*/
export const decodePackedArtifactWire = async (
	wire: ArrayBuffer,
): Promise<Record<string, unknown> | null> => artifactFrameFromWire(wire);
