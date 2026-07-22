import { describe, expect, it, vi } from "vitest";

const mockedReact = vi.hoisted(() => ({
	cleanup: undefined as (() => void) | undefined,
}));

vi.mock("react", async (importOriginal) => {
	const actual = await importOriginal<typeof import("react")>();

	return {
		...actual,
		useEffect: (effect: () => undefined | (() => void)) => {
			mockedReact.cleanup = effect() ?? undefined;
		},
	};
});

type WorkerListener = (event: MessageEvent) => void;

class MockWorker {
	static instances: MockWorker[] = [];

	messages: unknown[] = [];
	listeners: Record<string, WorkerListener[]> = {};

	constructor() {
		MockWorker.instances.push(this);
	}

	addEventListener(type: string, listener: WorkerListener) {
		this.listeners[type] = [...(this.listeners[type] ?? []), listener];
	}

	postMessage(message: unknown) {
		this.messages.push(message);
	}

	terminate() {}

	emit(type: string, data: unknown) {
		for (const listener of this.listeners[type] ?? []) {
			listener({ data } as MessageEvent);
		}
	}
}

vi.stubGlobal("Worker", MockWorker);

const { appStore } = await import("#/collections/app");
const { WsFeed } = await import("#/providers/websocket");

/*
WsFeed connects the worker and never publishes focus upstream — symbol selection
is a client paint concern only.
*/
describe("WsFeed", () => {
	it("connects without sending focus messages", () => {
		appStore.actions.updateFocusSymbol("BTC/USD");
		WsFeed();
		const worker = MockWorker.instances.at(-1);

		expect(worker).toBeDefined();
		worker?.emit("message", { type: "READY" });
		expect(worker?.messages).toContainEqual({
			type: "CONNECT",
			url: expect.any(String),
		});
		expect(
			worker?.messages.some(
				(message) =>
					typeof message === "object" &&
					message !== null &&
					"type" in message &&
					message.type === "FOCUS",
			),
		).toBe(false);

		appStore.actions.updateFocusSymbol("ETH/USD");
		expect(
			worker?.messages.some(
				(message) =>
					typeof message === "object" &&
					message !== null &&
					"type" in message &&
					message.type === "FOCUS",
			),
		).toBe(false);

		mockedReact.cleanup?.();
	});
});
