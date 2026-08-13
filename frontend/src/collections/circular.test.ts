import { describe, expect, it } from "vitest";
import { Circular } from "./circular";

describe("Circular", () => {
	it("exposes only populated values in logical order", () => {
		const buffer = Circular<number>(3);

		buffer.push(1);
		expect(buffer.array()).toEqual([1]);

		buffer.push(2);
		buffer.push(3);
		buffer.push(4);

		expect(buffer.array()).toEqual([2, 3, 4]);
		expect(buffer.values()).toEqual([2, 3, 4]);
		expect(buffer.length()).toBe(3);
		expect(buffer.capacity()).toBe(3);
	});
});
