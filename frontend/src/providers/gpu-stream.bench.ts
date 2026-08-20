import { bench, describe } from "vitest";
import { temporalLevel } from "./gpu-stream";

describe("GPU stream temporal selection", () => {
	bench("selects a level for a full 16K history", () => {
		temporalLevel(16_384, 1_440);
	});
});
