import { describe, expect, it } from "vitest";

import type { SignalGaugeBridge } from "#/components/charts/confidence/Gauges";
import {
	ingestGaugeWire,
	isGaugeWire,
} from "#/components/charts/confidence/gauge-wire";

describe("isGaugeWire", () => {
	it("accepts gauge chart frames", () => {
		expect(
			isGaugeWire({
				chart: "gauge",
				source: "hawkes",
				confidence: 1.2,
			}),
		).toBe(true);
	});
});

describe("ingestGaugeWire", () => {
	it("buffers wire frames until the gauge is ready", () => {
		const bridge: SignalGaugeBridge = {
			update: () => {},
			ready: false,
			pending: [],
			latest: {},
		};

		ingestGaugeWire(bridge, {
			chart: "gauge",
			source: "hawkes",
			confidence: 1.2,
			snr: 0.8,
		});

		expect(bridge.pending).toEqual([
			{
				chart: "gauge",
				source: "hawkes",
				confidence: 1.2,
				snr: 0.8,
			},
		]);
	});
});
