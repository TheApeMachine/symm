import { describe, expect, it } from "vitest";
import {
	meterInlineLabelClass,
	meterInlineValueClass,
	meterTrackVariants,
	meterVariants,
} from "./meter";
import type { Size } from "./types";

const meterSizes: Size[] = ["xxs", "xs", "s", "m", "lg", "xl", "xxl"];

describe("meterTrackVariants", () => {
	it("applies semantic tone and size classes to the track", () => {
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"[--meter-tone:var(--success)]",
		);
		expect(meterTrackVariants({ variant: "success", size: "m" })).toContain(
			"h-1.5",
		);
	});
});

describe("meterVariants", () => {
	it("applies layout classes", () => {
		expect(meterVariants({ layout: "inline" })).toBe("flex items-center gap-2");
	});
});

describe("meter inline typography", () => {
	it("defines label and value classes for every size", () => {
		for (const size of meterSizes) {
			expect(meterInlineLabelClass[size]).toMatch(/shrink-0/);
			expect(meterInlineLabelClass[size]).toMatch(/text-/);
			expect(meterInlineValueClass[size]).toMatch(
				/shrink-0 text-right font-mono/,
			);
			expect(meterInlineValueClass[size]).toMatch(/text-/);
		}
	});

	it("preserves existing s and m inline classes", () => {
		expect(meterInlineLabelClass.s).toBe(
			"w-[58px] shrink-0 text-[10px] text-(--f4)",
		);
		expect(meterInlineLabelClass.m).toBe(
			"w-[38px] shrink-0 font-mono text-[10px] text-(--f3)",
		);
		expect(meterInlineValueClass.s).toBe(
			"w-[18px] shrink-0 text-right font-mono text-[10px] text-(--f2)",
		);
		expect(meterInlineValueClass.m).toBe(
			"w-14 shrink-0 text-right font-mono text-[9px] text-(--f4)",
		);
	});
});
