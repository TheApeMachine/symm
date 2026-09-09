import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningRehearsalT } from "#/providers/telemetry/telemetry/learning-rehearsal";
import { learningFixture } from "./fixture";
import { RehearsalPanel } from "./rehearsal-panel";
import { projectLearning } from "./state";

describe("RehearsalPanel", () => {
	it("shows actual pool imbalance, selected counts and separate live results", () => {
		const state = learningFixture();
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "replaying",
			workers: 3,
			profitable: 12n,
			subfriction: 2n,
			declining: 5n,
			perWorker: 6n,
			decisions: 11n,
			trained: 10n,
			lastSymbol: "BTC/USD",
			lastAction: "enter",
			lastReturn: -0.02,
			lastFailure: "entry too early",
		});
		const view = projectLearning(state, "");
		expect(view.rehearsal).toBe(state.rehearsal);
		const html = renderToStaticMarkup(<RehearsalPanel view={view} />);
		expect(html).toContain("12 available, 2 selected per worker");
		expect(html).toContain("5 available, 2 selected per worker");
		expect(html).toContain("11 decisions graded → 10 absorbed");
		expect(html).toContain("-200.0 bp");
		expect(html).toContain("entry too early");
		expect(html).toContain("wallet P&amp;L -2");
		expect(html).toContain("captured precursor measurements");
	});

	it("keeps missing classes and unavailable economics visible", () => {
		const state = learningFixture();
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "waiting for gradeable episodes",
			workers: 2,
			ungraded: 7n,
		});
		const html = renderToStaticMarkup(
			<RehearsalPanel view={projectLearning(state, "")} />,
		);
		expect(html).toContain("Incomplete variety");
		expect(html).toContain("7 excluded");
		expect(html).toContain("0 available, 0 selected per worker");
		expect(html).not.toContain("Latest:");
	});

	it("does not invent worker state before telemetry arrives", () => {
		const html = renderToStaticMarkup(<RehearsalPanel view={null} />);
		expect(html).toContain("Historical workers have not reported");
		expect(html).not.toContain("0 historical workers");
	});
});
