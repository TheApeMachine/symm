import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";
import { flushBufferedFrames } from "./websocket";

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

	it("batches frame flushes by role and sends latest-only routes once", () => {
		const measurementBatches: Array<Array<Record<string, unknown>>> = [];
		const ticks: Array<Record<string, unknown>> = [];

		flushBufferedFrames(
			[
				{ role: "measurement", scope: "BTC/USD", origin: "fluid" },
				{ role: "tick", count: 1 },
				{ role: "measurement", scope: "ETH/USD", origin: "hawkes" },
				{ role: "tick", count: 2 },
			],
			{
				measurement: {
					batch: (frames) => measurementBatches.push(frames),
				},
				tick: {
					latest: (frame) => ticks.push(frame),
				},
			},
		);

		expect(measurementBatches).toEqual([
			[
				{ role: "measurement", scope: "BTC/USD", origin: "fluid" },
				{ role: "measurement", scope: "ETH/USD", origin: "hawkes" },
			],
		]);
		expect(ticks).toEqual([{ role: "tick", count: 2 }]);
	});
});
