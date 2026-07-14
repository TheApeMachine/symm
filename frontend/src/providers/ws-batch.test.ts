import { describe, expect, it, vi } from "vitest";
import { FrameBatcher } from "./ws-batch";

describe("FrameBatcher", () => {
	it("flushes coalesced payloads on the 16ms window", async () => {
		vi.useFakeTimers();
		const flushed: Record<string, unknown>[] = [];
		const batcher = new FrameBatcher((payload) => {
			flushed.push(payload);
		});

		batcher.enqueue({ tick: { count: 1 } });
		batcher.enqueue({ tick: { open: 2 } });
		batcher.enqueue({ positions: [{ symbol: "BTC/USD" }] });

		expect(flushed).toHaveLength(0);

		await vi.advanceTimersByTimeAsync(16);

		expect(flushed).toHaveLength(1);
		expect(flushed[0]).toEqual({
			tick: { count: 1, open: 2 },
			positions: [{ symbol: "BTC/USD" }],
		});

		batcher.dispose();
		vi.useRealTimers();
	});
});
