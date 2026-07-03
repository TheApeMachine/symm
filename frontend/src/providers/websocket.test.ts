import * as capnp from "capnp-ts";
import { describe, expect, it } from "vitest";

import { Circular } from "#/collections/circular";
import { measurementsStore } from "#/collections/measurements";
import { tickStore } from "#/collections/tick";
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

	it("routes decoded frames by role into stores without buffering", () => {
		measurementsStore.setState(() => ({
			measurements: {
				causal: Circular(50),
				correlation: Circular(50),
				cvd: Circular(50),
				depthflow: Circular(50),
				exhaustion: Circular(50),
				fluid: Circular(50),
				hawkes: Circular(50),
				leadlag: Circular(50),
				liquidity: Circular(50),
				manifold: Circular(50),
				pumpdump: Circular(50),
				resonance: Circular(50),
				sentiment: Circular(50),
				toxicity: Circular(50),
			},
			symbols: {},
		}));
		tickStore.actions.reset();

		routeFrame({ role: "measurement", scope: "BTC/USD", origin: "fluid" });
		routeFrame({ role: "tick", count: 1 });
		routeFrame({ role: "measurement", scope: "ETH/USD", origin: "hawkes" });
		routeFrame({ role: "tick", count: 2 });

		expect(measurementsStore.state.measurements.fluid.values()).toEqual([
			{ role: "measurement", scope: "BTC/USD", origin: "fluid" },
		]);
		expect(measurementsStore.state.measurements.hawkes.values()).toEqual([
			{ role: "measurement", scope: "ETH/USD", origin: "hawkes" },
		]);
		expect(tickStore.state.frame).toEqual({ role: "tick", count: 2 });
	});
});
