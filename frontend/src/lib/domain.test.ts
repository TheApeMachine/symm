import { describe, expect, it } from "vitest";
import {
	DomainError,
	requireNonZero,
	requirePositive,
	requirePositiveLength,
	requireSampleSize,
} from "./domain";

describe("domain", () => {
	describe("Given an empty sample", () => {
		it("should reject positive-length requirements", () => {
			expect(() => requirePositiveLength(0, "mean")).toThrow(DomainError);
			expect(() => requirePositiveLength(0, "mean")).toThrow(
				/sample length must be at least 1/,
			);
		});
	});

	describe("Given a singleton sample", () => {
		it("should reject n >= 2 estimators", () => {
			expect(() => requireSampleSize(1, 2, "spark")).toThrow(DomainError);
			expect(() => requireSampleSize(1, 2, "spark")).toThrow(/at least 2/);
		});
	});

	describe("Given a zero denominator", () => {
		it("should reject before division", () => {
			expect(() => requireNonZero(0, "span")).toThrow(DomainError);
			expect(requireNonZero(2, "span")).toBe(2);
		});
	});

	describe("Given a non-positive base", () => {
		it("should reject capital and price bases", () => {
			expect(() => requirePositive(0, "capital")).toThrow(DomainError);
			expect(() => requirePositive(-1, "capital")).toThrow(DomainError);
			expect(requirePositive(1.5, "capital")).toBe(1.5);
		});
	});
});
