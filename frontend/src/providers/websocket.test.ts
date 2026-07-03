import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Artifact } from "#/lib/capnp/artifact";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";
import { routeFrame } from "./websocket";

describe("WsFeed wire decode", () => {
	it("decodes packed hub measurement wire through read-artifact", async () => {
		const message = new capnp.Message();
		const artifact = message.initRoot(Artifact);

		artifact.setOrigin("pumpdump");
		artifact.setRole("measurement");
		artifact.setScope("BTC/USD");

		const attributeBytes = new TextEncoder().encode(JSON.stringify({}));
		artifact.initAttributes(attributeBytes.length).copyBuffer(attributeBytes);

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

	it("routes decoded frames by role without buffering", () => {
		const measurementBatches: Array<Array<Record<string, unknown>>> = [];
		const ticks: Array<Record<string, unknown>> = [];
		const routes = {
			measurement: {
				batch: (frames: Record<string, unknown>[]) =>
					measurementBatches.push(frames),
			},
			tick: {
				latest: (frame: Record<string, unknown>) => ticks.push(frame),
			},
		};

		routeFrame(
			{ role: "measurement", scope: "BTC/USD", origin: "fluid" },
			routes,
		);
		routeFrame({ role: "tick", count: 1 }, routes);
		routeFrame(
			{ role: "measurement", scope: "ETH/USD", origin: "hawkes" },
			routes,
		);
		routeFrame({ role: "tick", count: 2 }, routes);

		expect(measurementBatches).toEqual([
			[{ role: "measurement", scope: "BTC/USD", origin: "fluid" }],
			[{ role: "measurement", scope: "ETH/USD", origin: "hawkes" }],
		]);
		expect(ticks).toEqual([
			{ role: "tick", count: 1 },
			{ role: "tick", count: 2 },
		]);
	});
});
