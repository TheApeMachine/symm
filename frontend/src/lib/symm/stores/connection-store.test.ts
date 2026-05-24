import { describe, expect, it } from "vitest";

import {
	connectionStore,
	setFeedConnected,
} from "#/lib/symm/stores/connection-store";

describe("connection-store", () => {
	it("tracks one websocket feed connection", () => {
		connectionStore.setState(() => ({ connected: false }));

		setFeedConnected(true);
		expect(connectionStore.state.connected).toBe(true);

		setFeedConnected(true);
		expect(connectionStore.state.connected).toBe(true);

		setFeedConnected(false);
		expect(connectionStore.state.connected).toBe(false);
	});
});
