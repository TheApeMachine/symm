import { afterEach, describe, expect, it } from "vitest";
import {
	clearGraphResults,
	getGraphResults,
	setGraphResults,
} from "./graph-results.store";

afterEach(() => clearGraphResults("test"));

describe("setGraphResults", () => {
	it("replaces the last run without retaining missing upstream outputs", () => {
		setGraphResults("test", "revision", {
			first: { out: 3 },
			second: { out: 6 },
		});
		setGraphResults("test", "revision", { first: { out: 4 } });
		expect(getGraphResults("test", "revision")).toEqual({ first: { out: 4 } });
	});
	it("keeps undefined numeric results instead of fabricating zero", () => {
		setGraphResults("test", "revision", {
			division: { out: Number.POSITIVE_INFINITY },
		});
		expect(getGraphResults("test", "revision").division.out).toBe(
			Number.POSITIVE_INFINITY,
		);
	});
});

describe("getGraphResults", () => {
	it("hides an old response after graph edits", () => {
		setGraphResults("test", "old", { first: { out: 3 } });
		expect(getGraphResults("test", "new")).toEqual({});
		setGraphResults("test", "new", { first: { out: 8 } });
		expect(getGraphResults("test", "new")).toEqual({ first: { out: 8 } });
	});
});

describe("clearGraphResults", () => {
	it("removes the last successful result before another run", () => {
		setGraphResults("test", "revision", { first: { out: 3 } });
		clearGraphResults("test");
		expect(getGraphResults("test", "revision")).toEqual({});
	});
});
