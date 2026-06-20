import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact";
import { artifactFrameFromWire } from "#/lib/capnp/read-artifact";

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

		const wire = message.toArrayBuffer();
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

		const wire = message.toArrayBuffer();

		for (let index = 0; index < 1000; index += 1) {
			artifactFrameFromWire(wire);
		}
	});
});
