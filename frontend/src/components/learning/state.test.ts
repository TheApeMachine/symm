import { Builder } from "flatbuffers";
import { describe, expect, it } from "vitest";
import { learningStore, receiveLearning } from "#/collections/learning";
import { LearningDecisionT } from "#/providers/telemetry/telemetry/learning-decision";
import { learningFixture } from "./fixture";
import { projectLearning, updateLearningEvents } from "./state";

describe("projectLearning", () => {
	it("decodes real FlatBuffers into the same signed reading for the dashboard and toolbar", () => {
		const source = learningFixture();
		const builder = new Builder();
		builder.finish(source.pack(builder));
		receiveLearning(builder.asUint8Array());
		const view = projectLearning(learningStore.state!, "");
		expect(view.skill.mean).toBe(-0.005);
		expect(view.skill.defined).toBe(true);
		expect(view.lanes?.[0].profit).toBe(-2);
		expect(view.desk?.traders[0].wealth).toBe(-0.01);
		expect(view.resolved).toBe(3);
		expect(view.lanes?.[0].unresolved).toBe(17);
	});
	it("does not turn missing evidence into a measured zero", () => {
		const source = learningFixture();
		source.agents[0].reading = null;
		const view = projectLearning(source, "");
		expect(view.skill.defined).toBe(false);
	});
});

describe("updateLearningEvents", () => {
	it("retains distinct lanes, steps and event kinds while ignoring repeated snapshots", () => {
		const source = learningFixture();
		const decision = new LearningDecisionT();
		decision.id = source.steps;
		decision.symbol = "BTC/USD";
		decision.throughNs = source.atNs;
		source.agents = [0, 1, 2, 3, 4].map((id) =>
			Object.assign(learningFixture().agents[0], {
				id,
				last: decision,
				outcome: decision,
			}),
		);
		const first = updateLearningEvents([], source, "BTC/USD");
		expect(first).toHaveLength(10);
		const repeated = updateLearningEvents(first, source, "BTC/USD");
		expect(repeated).toEqual(first);
		expect(first).toHaveLength(10);

		source.steps += 1n;
		const advanced = updateLearningEvents(repeated, source, "BTC/USD");
		expect(advanced).toHaveLength(15);
		expect(advanced.filter((event) => event.kind === "resolved")).toHaveLength(
			5,
		);
		expect(updateLearningEvents(advanced, source, "BTC/USD")).toEqual(advanced);
	});

	it("filters valuations by symbol and retains the display budget across updates", () => {
		const source = learningFixture();
		source.agents[0].last = new LearningDecisionT();
		source.agents[0].last.symbol = "BTC/USD";
		expect(updateLearningEvents([], source, "ETH/USD")).toEqual([]);
		let events = updateLearningEvents([], source, "BTC/USD");

		for (let step = 0; step < 210; step += 1) {
			source.steps += 1n;
			events = updateLearningEvents(events, source, "BTC/USD");
		}

		expect(events).toHaveLength(200);
		expect(events[0].id).toBe(111);
		expect(events.at(-1)?.id).toBe(310);
		expect(updateLearningEvents(events, source, "BTC/USD")).toEqual(events);
	});
});
