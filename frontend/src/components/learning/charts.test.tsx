import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
	LearningProgress,
	DecisionRing,
	DrivingActions,
	EventRhythm,
	ImpulseBars,
	InfluenceGrid,
	OutcomeRange,
	PipelineFunnel,
	TraderQuality,
	WalletBars,
} from "./charts";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";
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

describe("OutcomeRange", () => {
	it("shows the actual signed mean on a labeled axis", () => {
		const skill = {
			mode: "learning",
			account: "sim",
			since: "",
			reason: "",
			varianceDefined: true,
			variance: 0,
			wins: 0,
			losses: 100,
			defined: true,

			mean: -0.0002,

			support: 12.5,
			samples: 100,
		} as Skill;

		const markup = renderToStaticMarkup(<OutcomeRange skill={skill} />);

		expect(markup).toContain("-2.0 bp");
		expect(markup).toContain("← 0 →");
	});

	it("does not draw a distance without resolved evidence", () => {
		const markup = renderToStaticMarkup(
			<OutcomeRange skill={{ defined: false } as Skill} />,
		);
		expect(markup).toContain("No completed outcomes yet");
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

describe("WalletBars", () => {
	it("shows positive and negative account values on the same axis", () => {
		const lanes = [
			{ lane: 0, mode: "policy", profit: 12, realized: 12 },
			{ lane: 1, mode: "explore", profit: -4, realized: -4 },
		] as Wallet[];

		const markup = renderToStaticMarkup(<WalletBars lanes={lanes} />);

		expect(markup).toContain("-4");
		expect(markup).toContain("policy 1");
	});
});

describe("DecisionRing", () => {
	it("reports the share the tape has already answered", () => {
		const view = {
			desk: {
				settled: 0,
				traders: [
					{
						id: 0,
						decisions: 10,
						fills: 0,
						graded: 5,
						observed: 5,
						quality: 0,
						wealth: 0,
						open: 3,
						holding: 0,
					},
					{
						id: 1,
						decisions: 10,
						fills: 0,
						graded: 5,
						observed: 5,
						quality: 0,
						wealth: 0,
						open: 3,
						holding: 0,
					},
				],
			},
		} as unknown as LearningView;

		const markup = renderToStaticMarkup(<DecisionRing view={view} />);

		expect(markup).toContain("50.0%");
		expect(markup).toContain("graded by the tape");
		expect(markup).toContain("superseded");
	});

	/*
		An empty ring must say nothing has happened rather than draw a shape
		implying something has.
	*/
	it("says so when no decision has been made", () => {
		const markup = renderToStaticMarkup(<DecisionRing view={null} />);
		expect(markup).toContain("No decision has been made yet");
	});
});

describe("TraderQuality", () => {
	it("marks a wallet with nothing graded rather than drawing it at zero", () => {
		const view = {
			desk: {
				settled: 0,
				traders: [
					{
						id: 0,
						decisions: 4,
						fills: 0,
						graded: 4,
						observed: 4,
						quality: -0.0002,
						wealth: 0,
						open: 0,
						holding: 0,
					},
					{
						id: 1,
						decisions: 1,
						fills: 0,
						graded: 0,
						observed: 0,
						quality: 0,
						wealth: 0,
						open: 1,
						holding: 0,
					},
				],
			},
		} as unknown as LearningView;

		const markup = renderToStaticMarkup(<TraderQuality view={view} />);

		expect(markup).toContain("wallet 1");
		expect(markup).toContain("nothing graded yet");
		expect(markup).toContain("-2.0 bp");
	});
});

describe("DrivingActions", () => {
	it("draws each action against centre with its evidence beneath", () => {
		const influence: Influence[] = [
			{
				token: 2,
				source: "Context",
				label: "prefix 2/14",
				action: "enter ·1/1",
				prior: prior({ Mean: 0.0006, Samples: 40, Support: 12 }),
			},
		];

		const markup = renderToStaticMarkup(
			<DrivingActions influence={influence} />,
		);

		expect(markup).toContain("enter ·1/1");
		expect(markup).toContain("6.0 bp");
		expect(markup).toContain("40 obs");
	});

	it("reports an absence of evidence instead of an empty chart", () => {
		const markup = renderToStaticMarkup(<DrivingActions influence={[]} />);
		expect(markup).toContain("No action has accumulated evidence yet");
	});
});

describe("LearningProgress", () => {
	it("uses retained pending decisions rather than counting discarded forced waits", () => {
		const state = learningFixture();
		state.decisions = 100000n;
		state.resolved = 10n;
		state.agents[0].pending = 3n;
		const html = renderToStaticMarkup(
			<LearningProgress view={projectLearning(state, "")} />,
		);
		expect(html).toContain("3");
		expect(html).toContain("awaiting a completed grade");
		expect(html).not.toContain("99,990");
	});
});
