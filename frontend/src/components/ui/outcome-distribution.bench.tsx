import { renderToStaticMarkup } from "react-dom/server";
import { bench, describe } from "vitest";
import { OutcomeDistribution } from "./outcome-distribution";
// Explicit fixture bounds and counts; no distribution is fitted by the view.
const bins = Array.from({ length: 64 }, (_, index) => ({
	id: String(index),
	lower: index - 32,
	upper: index - 31,
	count: index,
}));
describe("OutcomeDistribution", () => {
	bench("renders 64 supplied bins", () => {
		renderToStaticMarkup(<OutcomeDistribution bins={bins} />);
	});
});
