// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeAtom } from "#/collections/app";
import { WsFeed } from "./websocket";

class FeedWorker {
	static latest: FeedWorker;
	postMessage = vi.fn();
	addEventListener = vi.fn();
	terminate = vi.fn();

	constructor() {
		FeedWorker.latest = this;
	}
}

afterEach(() => {
	cleanup();
	routeAtom.set("dashboard");
	vi.unstubAllGlobals();
});

describe("WsFeed", () => {
	it("seeds the current route before connecting and forwards subsequent navigation", () => {
		vi.stubGlobal("Worker", FeedWorker);
		routeAtom.set("xray");
		render(<WsFeed />);
		const worker = FeedWorker.latest;
		expect(worker.postMessage.mock.calls[0][0]).toEqual({
			type: "ROUTE",
			route: "xray",
		});
		expect(worker.postMessage.mock.calls[1][0]).toMatchObject({
			type: "CONNECT",
		});
		act(() => routeAtom.set("fluid"));
		expect(worker.postMessage).toHaveBeenLastCalledWith({
			type: "ROUTE",
			route: "fluid",
		});
	});
});
