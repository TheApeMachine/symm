import { LearningDecisionT } from "#/providers/telemetry/telemetry/learning-decision";
import { bench } from "vitest";
import { learningFixture } from "./fixture";
import {
	projectLearning,
	updateLearningEvents,
	type LearningEvent,
} from "./state";

import { LearningDevelopmentT } from "#/providers/telemetry/telemetry/learning-development";
import { LearningQuantityT } from "#/providers/telemetry/telemetry/learning-quantity";
const source = learningFixture();
source.agents = Array.from({ length: 8 }, (_, id) =>
	Object.assign(learningFixture().agents[0], { id }),
);
source.markets = Array.from({ length: 450 }, (_, index) => {
	const market = new LearningDevelopmentT();
	market.symbol = `symbol-${index}`;
	market.atNs = source.atNs;
	market.fromNs = 1n;
	if (index === 0)
		market.quantities = Array.from({ length: 405 }, (_, column) =>
			Object.assign(new LearningQuantityT(), {
				source: "signal",
				label: `metric-${column}`,
				present: true,
				activity: column / 405,
			}),
		);
	return market;
});
bench("projectLearning eight agents and 450 markets", () => {
	projectLearning(source, "");
});

// Exercise the full display budget with eight simultaneous account valuations.
let events: LearningEvent[] = [];
for (const member of source.agents) {
	member.last = new LearningDecisionT();
	member.last.symbol = "BTC/USD";
}
for (let step = 0; step < 25; step += 1) {
	source.steps += 1n;
	events = updateLearningEvents(events, source, "");
}
bench("updateLearningEvents repeated snapshot at display capacity", () => {
	updateLearningEvents(events, source, "");
});
bench("updateLearningEvents advancing eight agents at display capacity", () => {
	source.steps += 1n;
	events = updateLearningEvents(events, source, "");
});
