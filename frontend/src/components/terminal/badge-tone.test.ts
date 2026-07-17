import { describe, expect, it } from "vitest";
import { paletteGroupVariant, verdictToVariant } from "./badge-tone";

describe("verdictToVariant", () => {
	it("maps candidate verdicts to badge variants", () => {
		expect(verdictToVariant("allow")).toBe("success");
		expect(verdictToVariant("blocked")).toBe("error");
		expect(verdictToVariant("hold")).toBe("warning");
		expect(verdictToVariant("waiting")).toBe("info");
		expect(verdictToVariant("below")).toBe("info");
	});
});

describe("paletteGroupVariant", () => {
	it("maps palette groups to badge variants", () => {
		expect(paletteGroupVariant("Surface")).toBe("info");
		expect(paletteGroupVariant("Symbol")).toBe("success");
		expect(paletteGroupVariant("Kernel")).toBe("warning");
	});
});
