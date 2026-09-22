import { bench, describe } from "vitest";
import {
	clearGraphResults,
	getGraphResults,
	setGraphResults,
} from "./graph-results.store";

const results = { add: { out: 5 }, square: { out: 25 } };

describe("setGraphResults", () => {
	bench("publishes and reads a versioned backend run", () => {
		setGraphResults("benchmark", "revision", results);
		getGraphResults("benchmark", "revision");
		clearGraphResults("benchmark");
	});
});
