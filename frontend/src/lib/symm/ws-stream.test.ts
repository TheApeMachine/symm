import { afterEach, describe, expect, it, vi } from "vitest";

import type { SignalScoreEvent } from "#/lib/symm/events";
import { WsStream } from "#/lib/symm/ws-stream";

type MockSocket = {
	readyState: number;
	onopen: (() => void) | null;
	onclose: (() => void) | null;
	onerror: (() => void) | null;
	onmessage: ((message: { data: string }) => void) | null;
	send: ReturnType<typeof vi.fn>;
	close: ReturnType<typeof vi.fn>;
};

const OPEN = 1;

describe("WsStream", () => {
	let lastSocket: MockSocket | null = null;

	afterEach(() => {
		lastSocket = null;
		vi.unstubAllGlobals();
	});

	it("uses one socket and dispatches parsed events", () => {
		const events: string[] = [];

		vi.stubGlobal(
			"WebSocket",
			vi.fn(function WebSocket() {
				lastSocket = {
					readyState: OPEN,
					onopen: null,
					onclose: null,
					onerror: null,
					onmessage: null,
					send: vi.fn(),
					close: vi.fn(),
				};

				return lastSocket;
			}),
		);

		const stream = new WsStream({
			url: "ws://127.0.0.1:8765/ws",
			onEvent: (event) => {
				events.push(event.event);
			},
		});

		stream.start();
		lastSocket?.onopen?.();

		const payload: SignalScoreEvent = {
			event: "signal_score",
			ts: "2026-05-23T12:00:00Z",
			source: "hawkes",
			confidence: 0.5,
		};

		lastSocket?.onmessage?.({ data: JSON.stringify(payload) });

		expect(globalThis.WebSocket).toHaveBeenCalledTimes(1);
		expect(events).toEqual(["signal_score"]);

		stream.stop();
		expect(lastSocket?.close).toHaveBeenCalled();
	});
});
