import { bench } from "vitest";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

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
