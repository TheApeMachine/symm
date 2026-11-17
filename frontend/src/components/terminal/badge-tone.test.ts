import { describe, expect, it } from "vitest";
import { paletteGroupVariant } from "./badge-tone";

describe("paletteGroupVariant", () => {
	it("maps palette groups to badge variants", () => {
		expect(paletteGroupVariant("Surface")).toBe("info");
		expect(paletteGroupVariant("Symbol")).toBe("success");
		expect(paletteGroupVariant("Other")).toBe("warning");
	});
});