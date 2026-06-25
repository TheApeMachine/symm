import { describe, expect, it } from "vitest";
import { parseSurface } from "#/components/terminal/symm-terminal";

describe("parseSurface", () => {
	it.each([
		["insight", "signals"],
		["decision", "decisions"],
		["alloc", "allocation"],
		["signals", "signals"],
		["dashboard", "dashboard"],
	])("maps legacy surface alias %s to %s", (input, expected) => {
		expect(parseSurface(input)).toBe(expected);
	});

	it("falls back to dashboard for unknown surfaces", () => {
		expect(parseSurface("unknown")).toBe("dashboard");
		expect(parseSurface(undefined)).toBe("dashboard");
	});
});
