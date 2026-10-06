import { describe, expect, it } from "vitest";
import { outcome, rational } from "./format";

describe("rational", () => {
	it("formats exact account fractions only at the display boundary", () => {
		expect(rational("150")).toBe("150");
		expect(rational("4799/100")).toBe("47.99");
		expect(rational("0")).toBe("0");
		expect(rational("")).toBe("unavailable");
	});
});

describe("outcome", () => {
	it("names all five excursion classes from direction or type code", () => {
		expect(outcome("up", 0)).toBe("UP");
		expect(outcome("up_friction", 0)).toBe("UP_FRICTION");
		expect(outcome("down", 0)).toBe("DOWN");
		expect(outcome("chop", 0)).toBe("CHOP");
		expect(outcome("flat", 0)).toBe("FLAT");
		expect(outcome("", 1)).toBe("UP");
		expect(outcome("", 2)).toBe("DOWN");
		expect(outcome("", 3)).toBe("CHOP");
		expect(outcome("", 4)).toBe("FLAT");
		expect(outcome("", 5)).toBe("UP_FRICTION");
	});
});
