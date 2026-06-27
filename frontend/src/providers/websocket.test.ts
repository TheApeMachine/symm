import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

describe("WsFeed wire decode", () => {
	it("decodes packed hub measurement wire through read-artifact", async () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setOrigin("pumpdump");
		artifact.setRole("measurement");
		artifact.setScope("BTC/USD");

		const payloadBytes = new TextEncoder().encode(
			JSON.stringify({
				samples: 60,
				calibrated: true,
				output: { confidence: 0.73 },
			}),
		);
		artifact.initPayload(payloadBytes.length).copyBuffer(payloadBytes);

		const frame = await decodePackedArtifactWire(message.toPackedArrayBuffer());

		expect(frame).not.toBeNull();
		expect(frame?.origin).toBe("pumpdump");
		expect(frame?.role).toBe("measurement");
		expect(frame?.samples).toBe(60);
		expect(frame?.calibrated).toBe(true);

		const output = frame?.output as Record<string, unknown>;

		expect(output.confidence).toBeGreaterThan(0);
	});
});
