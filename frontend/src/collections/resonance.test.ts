import { beforeEach, describe, expect, it } from "vitest";
import { appStore } from "#/collections/app";
import { resonanceStore } from "./resonance";

describe("resonanceStore", () => {
	beforeEach(() => {
		appStore.actions.updateFocusSymbol("BTC/USD");
		resonanceStore.setState(() => ({
			resonance: {},
			version: 0,
		}));
	});

	it("keeps only the latest latent frame outside the focused symbol", () => {
		resonanceStore.actions.updateFrame({
			source: "resonance",
			symbol: "ETH/USD",
			at: "2026-07-12T00:00:00Z",
			samples: 1,
		});
		resonanceStore.actions.updateFrame({
			source: "resonance",
			symbol: "ETH/USD",
			at: "2026-07-12T00:00:01Z",
			samples: 2,
		});

		expect(resonanceStore.state.resonance["ETH/USD"]?.capacity()).toBe(1);
		expect(resonanceStore.state.resonance["ETH/USD"]?.values()).toHaveLength(1);
		expect(
			resonanceStore.state.resonance["ETH/USD"]?.values().at(-1)?.samples,
		).toBe(2);
	});

	it("accepts the singleton domain object published by the backend", () => {
		resonanceStore.actions.updateFrame({
			source: "resonance",
			symbol: "RESONANCE/TEST",
			at: "2026-07-12T00:00:00Z",
			samples: 1,
		});

		expect(
			resonanceStore.state.resonance["RESONANCE/TEST"]?.values().at(-1),
		).toMatchObject({ source: "resonance", samples: 1 });
		expect(resonanceStore.state.version).toBe(1);
	});

	it("retains every publish frame even when the observation timestamp is unchanged", () => {
		const frame = (at: string, samples: number) => ({
			source: "resonance",
			symbol: "BTC/USD",
			at,
			samples,
			surprise: 0.25,
			layers: [{ state: [samples], prediction: [samples + 0.5] }],
		});

		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:00Z", 1));
		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:00Z", 2));
		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:01Z", 3));

		expect(resonanceStore.state.resonance["BTC/USD"]?.values()).toEqual([
			frame("2026-07-12T00:00:00Z", 1),
			frame("2026-07-12T00:00:00Z", 2),
			frame("2026-07-12T00:00:01Z", 3),
		]);
	});
});
