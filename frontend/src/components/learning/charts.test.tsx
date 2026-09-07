import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
	EventRhythm,
	ExposureRing,
	ImpulseBars,
	InfluenceGrid,
	PipelineFunnel,
	PromotionLadder,
	WalletBars,
} from "./charts";
import type {
	Influence,
	LearningEvent,
	LearningView,
	Prior,
	Skill,
	Token,
	Wallet,
} from "./state";

const prior = (over: Partial<Prior>): Prior => ({
	Samples: 10,
	Defined: true,
	Mean: 0.0004,
	Variance: 0.00001,
	VarianceDefined: true,
	Support: 6,
	Maturity: 0.5,
	Authority: 0.5,
	...over,
});

describe("PipelineFunnel", () => {
	it("names every stage and the fraction that survives the step above it", () => {
		const view = {
			steps: 10000,
			decisions: 1000,
			dispatched: 0,
			resolved: 40,
			hasExecution: false,
			execution: {
				submitted: 0,
				unsupported: 0,
				diverged: 0,
				dropped: 0,
				failed: 0,
				queued: 0,
			},
		} as unknown as LearningView;

		const markup = renderToStaticMarkup(<PipelineFunnel view={view} />);

		expect(markup).toContain("Market observations");
		expect(markup).toContain("Outcomes measured");
		expect(markup).toContain("10.0% of the step above");
		expect(markup).toContain("Nothing reaches this step yet");
	});
});

describe("EventRhythm", () => {
	it("reports an empty journal rather than drawing an empty axis", () => {
		const markup = renderToStaticMarkup(<EventRhythm events={[]} />);
		expect(markup).toContain("The journal is empty");
	});

	it("draws one row per recorded kind", () => {
		const events = [
			{
				id: 1,
				lane: 0,
				mode: "policy",
				kind: "issued",
				at: "2026-09-07T12:00:00Z",
				action: "buy",
				power: 0,
				reduce: false,
				cash: "1",
				inventory: "0",
				authority: 1,
				profit: 0,
				complete: false,
				episode: 1,
				horizonNs: 1e9,
				prior: prior({}),
			},
			{
				id: 2,
				lane: 0,
				mode: "policy",
				kind: "resolved",
				at: "2026-09-07T12:00:10Z",
				action: "buy",
				power: 0,
				reduce: false,
				cash: "1",
				inventory: "0",
				authority: 1,
				profit: 0,
				complete: true,
				episode: 1,
				horizonNs: 1e9,
				prior: prior({}),
			},
		] as LearningEvent[];

		const markup = renderToStaticMarkup(<EventRhythm events={events} />);

		expect(markup).toContain("issued");
		expect(markup).toContain("resolved");
		expect(markup).toContain("2 recorded moments");
	});
});

describe("PromotionLadder", () => {
	it("states the distance still to cover when the bound is below the line", () => {
		const skill = {
			defined: true,
			qualified: true,
			mean: -0.0002,
			lowerBound: -0.0005,
			sigma: 3,
			support: 12.5,
			samples: 100,
		} as Skill;

		const markup = renderToStaticMarkup(<PromotionLadder skill={skill} />);

		expect(markup).toContain("Short of the line by 5.0 bp");
		expect(markup).toContain("12.5 of 100");
	});

	it("does not draw a distance without resolved evidence", () => {
		const markup = renderToStaticMarkup(
			<PromotionLadder skill={{ defined: false } as Skill} />,
		);
		expect(markup).toContain("No resolved evidence yet");
	});
});

describe("ImpulseBars", () => {
	it("orders the hot quantities by the strength it draws", () => {
		const impulse: Token[] = [
			{
				token: 3,
				source: "cvd",
				label: "divergence",
				strength: 4,
				authority: 0.5,
				members: 2,
			},
		];

		const markup = renderToStaticMarkup(<ImpulseBars impulse={impulse} />);

		expect(markup).toContain("cvd");
		expect(markup).toContain("divergence");
	});
});

describe("InfluenceGrid", () => {
	it("draws a dashed cell for a pairing that has never resolved", () => {
		const influence: Influence[] = [
			{
				token: 1,
				source: "cvd",
				label: "divergence",
				action: "buy",
				prior: prior({ Mean: 0.0006 }),
			},
			{
				token: 2,
				source: "morphology",
				label: "concentration",
				action: "sell",
				prior: prior({ Mean: -0.0003 }),
			},
		];

		const markup = renderToStaticMarkup(
			<InfluenceGrid influence={influence} />,
		);

		expect(markup).toContain("never resolved here");
		expect(markup).toContain("morphology");
	});
});

describe("ExposureRing", () => {
	it("reports the share of confirmed excursions the policy held through", () => {
		const markup = renderToStaticMarkup(
			<ExposureRing
				forward={{
					reviewed: 4,
					exposed: 1,
					unexposed: 2,
					captured: 1,
					missed: 2,
					unreviewable: 1,
					at: "2026-09-07T12:00:00Z",
					recent: null,
				}}
			/>,
		);

		expect(markup).toContain("25.0%");
		expect(markup).toContain("sat it out");
	});
});

describe("WalletBars", () => {
	it("leaves an unvalued wallet as a gap rather than a bar at zero", () => {
		const lanes = [
			{ lane: 0, mode: "policy", profit: 12, realized: 12, complete: true },
			{ lane: 1, mode: "explore", profit: 0, realized: 0, complete: false },
		] as Wallet[];

		const markup = renderToStaticMarkup(<WalletBars lanes={lanes} />);

		expect(markup).toContain("not valued yet");
		expect(markup).toContain("policy 1");
	});
});
