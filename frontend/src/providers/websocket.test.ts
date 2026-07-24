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
WsFeed connects the worker and publishes focus upstream so signal metrics can
gate on the selected symbol.
*/
describe("WsFeed", () => {
	it("connects and sends focus messages on ready and focus changes", () => {
		appStore.actions.updateFocusSymbol("BTC/USD");
		WsFeed();
		const worker = MockWorker.instances.at(-1);

		expect(worker).toBeDefined();
		worker?.emit("message", { type: "READY" });
		expect(worker?.messages).toContainEqual({
			type: "CONNECT",
			url: expect.any(String),
		});
		expect(worker?.messages).toContainEqual({
			type: "FOCUS",
			symbol: "BTC/USD",
		});

		appStore.actions.updateFocusSymbol("ETH/USD");
		expect(worker?.messages).toContainEqual({
			type: "FOCUS",
			symbol: "ETH/USD",
		});

		mockedReact.cleanup?.();
	});
});
