import { bench } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { EdgeDistributionPlot } from "./visualizer";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

const state = learningFixture();
state.agents[0].reading!.varianceDefined = true;
state.agents[0].reading!.variance = 0.000002;
const view = projectLearning(state, "");

bench("EdgeDistributionPlot normal fit from measured moments", () => {
	renderToStaticMarkup(<EdgeDistributionPlot skill={view.skill} />);
});
