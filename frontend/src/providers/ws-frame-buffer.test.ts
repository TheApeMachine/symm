import { describe, expect, it } from "vitest";
import { WorkerFrameBuffer } from "#/providers/ws-frame-buffer";

describe("WorkerFrameBuffer", () => {
	it("keeps only the latest sparse delta per identity until take", () => {
		const buffer = new WorkerFrameBuffer();

		buffer.merge({
			positions: [{ id: "btc", status: "open" }],
			cognition: { "BTC/USD": { winnerRegime: "coil" } },
			resonance: [{ symbol: "BTC/USD", confidence: 0.5 }],
		});
		buffer.merge({
			positions: [{ id: "btc", status: "closed" }],
			cognition: {
				"BTC/USD": { winnerRegime: "flush" },
				"ETH/USD": { winnerRegime: "lift" },
			},
			resonance: [
				{ symbol: "ETH/USD", confidence: 0.25 },
				{ symbol: "BTC/USD", confidence: 0.75 },
			],
		});

		expect(buffer.take()).toEqual({
			positions: [{ id: "btc", status: "closed" }],
			cognition: {
				"BTC/USD": { winnerRegime: "flush" },
				"ETH/USD": { winnerRegime: "lift" },
			},
			resonance: [
				{ symbol: "BTC/USD", confidence: 0.75 },
				{ symbol: "ETH/USD", confidence: 0.25 },
			],
		});
		expect(buffer.take()).toBeNull();
	});
});
