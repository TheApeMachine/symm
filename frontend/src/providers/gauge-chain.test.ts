import { describe, expect, it } from "vitest";
import { parseGaugeFrame, isSignalDiagnosticReading } from "#/collections/signals";

// The exact decrypted payload the backend produces, plus the capnp identity
// websocket.ts attaches (origin = signal name, role = measurement, scope).
const backendFrame = {
	channel: "book",
	calibrated: true,
	type: "update",
	data: [{ symbol: "BTC/USD" }],
	samples: 1,
	output: {
		strength: 0,
		probabilities: [0.3333, 0.3333, 0.3333],
		value: 1,
		category: 1,
		confidence: 0.3333333333333333,
	},
	role: "measurement",
	scope: "toxicity",
	origin: "toxicity",
	destination: "ui",
};

describe("backend measurement -> gauge reading", () => {
	it("parses source and confidence from a real backend frame", () => {
		const reading = parseGaugeFrame(backendFrame);
		expect(reading).not.toBeNull();
		expect(reading?.source).toBe("toxicity");
		expect(reading?.confidence).toBeCloseTo(0.3333, 3);
		expect(reading?.surprise).toBe(1);
		expect(reading?.category).toBe("1");
	});

	it("is accepted as a diagnostic reading (so the gauge updates)", () => {
		const reading = parseGaugeFrame(backendFrame);
		expect(reading).not.toBeNull();
		expect(isSignalDiagnosticReading(reading!)).toBe(true);
	});
});
