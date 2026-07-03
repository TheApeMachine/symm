import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact";
import {
	artifactFrameFromWire,
	decodePackedArtifactWire,
} from "#/lib/capnp/read-artifact";

describe("decodePackedArtifactWire", () => {
	it("decodes attributes and identity fields from packed wire", async () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setRole("measurement");
		artifact.setScope("trader");
		artifact.setOrigin("toxicity");
		artifact.setDestination("ui");
		const attributeBytes = new TextEncoder().encode(
			JSON.stringify({ output: 0.42 }),
		);
		artifact.initAttributes(attributeBytes.length).copyBuffer(attributeBytes);

		const wire = message.toPackedArrayBuffer();
		const frame = await decodePackedArtifactWire(wire);

		expect(frame).toEqual({
			output: 0.42,
			origin: "toxicity",
			scope: "trader",
			role: "measurement",
			destination: "ui",
		});
	});

	it("exposes artifact UnixNano timestamp as dashboard observed time", async () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setRole("measurement");
		artifact.setScope("update");
		artifact.setOrigin("pumpdump");
		artifact.setTimestamp(
			capnp.Int64.fromNumber(1_766_666_666_123_000) as unknown as bigint,
		);

		const wire = message.toPackedArrayBuffer();
		const frame = await decodePackedArtifactWire(wire);

		expect(frame?.timestamp_unix_nano).toBe("1766666666123000");
		expect(frame?.observed_at).toBe(1766666666);
	});

	it("decodes plaintext payload from the schema payload field", async () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setRole("decision");
		artifact.setScope("BTC/USD");
		artifact.setOrigin("trader");
		const payloadBytes = new TextEncoder().encode(
			JSON.stringify({ verdict: "entry", edge: 0.91 }),
		);
		artifact.initPayload(payloadBytes.length).copyBuffer(payloadBytes);

		const wire = message.toPackedArrayBuffer();
		const frame = await decodePackedArtifactWire(wire);

		expect(frame).toMatchObject({
			role: "decision",
			scope: "BTC/USD",
			origin: "trader",
			verdict: "entry",
			edge: 0.91,
		});
	});

	it("returns null for malformed wire buffers", async () => {
		const malformed = Uint8Array.from([1, 2, 3]).buffer;

		expect(await decodePackedArtifactWire(malformed)).toBeNull();
	});

	it("returns null for empty wire buffers", async () => {
		expect(await decodePackedArtifactWire(new ArrayBuffer(0))).toBeNull();
	});
});

describe("artifactFrameFromWire", () => {
	it("decodes measurement wire frames with attributes", () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setRole("measurement");
		artifact.setScope("trader");
		artifact.setOrigin("toxicity");
		artifact.setDestination("ui");
		const attributeBytes = new TextEncoder().encode(
			JSON.stringify({ output: 0.42 }),
		);
		artifact.initAttributes(attributeBytes.length).copyBuffer(attributeBytes);

		const wire = message.toPackedArrayBuffer();
		const frame = artifactFrameFromWire(wire);

		expect(frame).toEqual({
			output: 0.42,
			origin: "toxicity",
			scope: "trader",
			role: "measurement",
			destination: "ui",
		});
	});

	it("returns null for empty wire buffers", () => {
		expect(artifactFrameFromWire(new ArrayBuffer(0))).toBeNull();
	});

	it("returns null for malformed wire buffers", () => {
		const malformed = Uint8Array.from([1, 2, 3]).buffer;

		expect(artifactFrameFromWire(malformed)).toBeNull();
	});
});

describe("artifactFrameFromWire benchmark", () => {
	it("benchmark", () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setRole("measurement");
		artifact.setScope("trader");
		artifact.setOrigin("toxicity");
		artifact.setDestination("ui");
		const attributeBytes = new TextEncoder().encode(
			JSON.stringify({ output: 0.42 }),
		);
		artifact.initAttributes(attributeBytes.length).copyBuffer(attributeBytes);

		const wire = message.toPackedArrayBuffer();

		for (let index = 0; index < 1000; index += 1) {
			artifactFrameFromWire(wire);
		}
	});
});
