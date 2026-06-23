import { beforeEach, describe, expect, it } from "vitest";

import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
	parseCognitiveFrame,
	selectedCognitiveReading,
} from "#/collections/cognitive";

const sampleReading = (): CognitiveReading => ({
	scope: "BTC/USD",
	sequence: "measurement/BTC/USD/fluid",
	regimePrefix: "regime/BTC",
	regimeCohort: 2,
	ambiguous: true,
	sideline: true,
	entropyBits: 1.5,
	entropyThreshold: 2,
	classConfidence: 0.82,
	contrastEvidence: 0.41,
	lookaheadScore: 0.67,
	lookaheadPaths: 3,
	winnerClass: "laminar",
	prewarmPaths: null,
	prewarmScore: null,
	updatedAt: Date.now(),
});

describe("parseCognitiveFrame", () => {
	it("returns null when scope is missing", () => {
		expect(parseCognitiveFrame({ type: "cognitive" })).toBeNull();
		expect(parseCognitiveFrame({ scope: "" })).toBeNull();
		expect(parseCognitiveFrame({ scope: "   " })).toBeNull();
	});

	it("normalizes cognitive websocket payloads", () => {
		const reading = parseCognitiveFrame({
			type: "cognitive",
			scope: "BTC/USD",
			sequence: "measurement/BTC/USD/fluid",
			regime_prefix: "regime/BTC",
			regime_cohort: 2,
			ambiguous: true,
			sideline: true,
			entropy_bits: 1.5,
			entropy_threshold: 2,
			class_confidence: 0.82,
			contrast_evidence: 0.41,
			lookahead_score: 0.67,
			lookahead_paths: 3,
			winner_class: "laminar",
		});

		expect(reading).not.toBeNull();
		expect(reading?.scope).toBe("BTC/USD");
		expect(reading?.sequence).toBe("measurement/BTC/USD/fluid");
		expect(reading?.regimePrefix).toBe("regime/BTC");
		expect(reading?.regimeCohort).toBe(2);
		expect(reading?.ambiguous).toBe(true);
		expect(reading?.sideline).toBe(true);
		expect(reading?.entropyBits).toBe(1.5);
		expect(reading?.entropyThreshold).toBe(2);
		expect(reading?.classConfidence).toBe(0.82);
		expect(reading?.contrastEvidence).toBe(0.41);
		expect(reading?.lookaheadScore).toBe(0.67);
		expect(reading?.lookaheadPaths).toBe(3);
		expect(reading?.winnerClass).toBe("laminar");
		expect(reading?.prewarmPaths).toBeNull();
		expect(reading?.prewarmScore).toBeNull();
	});

	it("accepts optional prewarm telemetry", () => {
		const reading = parseCognitiveFrame({
			scope: "ETH/USD",
			prewarm_paths: 5,
			prewarm_score: 0.33,
		});

		expect(reading).not.toBeNull();
		expect(reading?.prewarmPaths).toBe(5);
		expect(reading?.prewarmScore).toBe(0.33);
	});

	it("coerces invalid numeric fields to safe defaults", () => {
		const reading = parseCognitiveFrame({
			scope: "SOL/USD",
			regime_cohort: -3.7,
			entropy_bits: "bad",
			lookahead_paths: Number.NaN,
		});

		expect(reading).not.toBeNull();
		expect(reading?.regimeCohort).toBe(0);
		expect(reading?.entropyBits).toBe(0);
		expect(reading?.lookaheadPaths).toBe(0);
	});

	it("only treats explicit true as boolean flags", () => {
		const reading = parseCognitiveFrame({
			scope: "BTC/USD",
			ambiguous: 1,
			sideline: "yes",
		});

		expect(reading).not.toBeNull();
		expect(reading?.ambiguous).toBe(false);
		expect(reading?.sideline).toBe(false);
	});

	it("normalizes nested cognition attributes from measurement artifacts", () => {
		const reading = parseCognitiveFrame({
			role: "measurement",
			origin: "fluid",
			scope: "BTC/USD",
			cognition: {
				surprise: { value: 2.4, threshold: 1.8 },
				ambiguity: { bits: 1.5, threshold: 2, ambiguous: true },
				classification: {
					highest: 0.82,
					divergence: 0.41,
					winner: "laminar",
				},
				lookahead: { score: 0.67, paths: 3 },
				sequence: {
					value: "BTC/USD_fluid_measurement",
					regime: { prefix: "BTC/USD_fluid", cohort: 2 },
				},
			},
		});

		expect(reading).not.toBeNull();
		expect(reading?.scope).toBe("BTC/USD");
		expect(reading?.sequence).toBe("BTC/USD_fluid_measurement");
		expect(reading?.regimePrefix).toBe("BTC/USD_fluid");
		expect(reading?.regimeCohort).toBe(2);
		expect(reading?.ambiguous).toBe(true);
		expect(reading?.entropyBits).toBe(1.5);
		expect(reading?.entropyThreshold).toBe(2);
		expect(reading?.classConfidence).toBe(0.82);
		expect(reading?.contrastEvidence).toBe(0.41);
		expect(reading?.lookaheadScore).toBe(0.67);
		expect(reading?.lookaheadPaths).toBe(3);
		expect(reading?.winnerClass).toBe("laminar");
	});

	it("uses cognition.sequence.scope when artifact scope is absent", () => {
		const reading = parseCognitiveFrame({
			cognition: {
				sequence: {
					scope: "ETH/USD",
					value: "ETH/USD_ticker_update",
				},
			},
		});

		expect(reading).not.toBeNull();
		expect(reading?.scope).toBe("ETH/USD");
		expect(reading?.sequence).toBe("ETH/USD_ticker_update");
	});
});

describe("cognitiveStore", () => {
	beforeEach(() => {
		cognitiveStore.setState({ readings: {}, selectedScope: "" });
	});

	it("stores readings by scope", () => {
		const reading = sampleReading();

		cognitiveStore.actions.updateReading(reading);

		expect(cognitiveStore.state.readings["BTC/USD"]).toEqual(reading);
	});

	it("selects the first sealed scope automatically", () => {
		cognitiveStore.actions.updateReading(sampleReading());

		expect(cognitiveStore.state.selectedScope).toBe("BTC/USD");
	});

	it("keeps the selected scope when additional readings arrive", () => {
		cognitiveStore.actions.updateReading(sampleReading());
		cognitiveStore.actions.updateReading({
			...sampleReading(),
			scope: "ETH/USD",
		});

		expect(cognitiveStore.state.selectedScope).toBe("BTC/USD");
	});

	it("updates selected scope on demand", () => {
		cognitiveStore.actions.updateReading(sampleReading());
		cognitiveStore.actions.updateReading({
			...sampleReading(),
			scope: "ETH/USD",
		});
		cognitiveStore.actions.selectScope("ETH/USD");

		expect(cognitiveStore.state.selectedScope).toBe("ETH/USD");
		expect(selectedCognitiveReading()?.scope).toBe("ETH/USD");
	});

	it("lists scopes in sorted order", () => {
		cognitiveStore.actions.updateReading({
			...sampleReading(),
			scope: "ETH/USD",
		});
		cognitiveStore.actions.updateReading(sampleReading());

		expect(cognitiveScopes()).toEqual(["BTC/USD", "ETH/USD"]);
	});

	it("returns null when no scope is selected", () => {
		expect(selectedCognitiveReading()).toBeNull();
	});
});
