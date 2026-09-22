import { bench, describe } from "vitest";
import { computeSparklinePaths } from "./sparkline";

// A fixture spanning a viewport, including values beyond the old unit range.
const values = Array.from(
	{ length: 150 },
	(_, index) => 100 + 20 * Math.sin(index / 10),
);
describe("computeSparklinePaths", () => {
	bench("scales and draws 150 supplied observations", () => {
		computeSparklinePaths(values);
	});
});
