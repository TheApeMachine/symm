import { describe, expect, it } from "vitest";
import { appStore } from "#/collections/app";
import { applyFramePayload } from "#/providers/ws-stores";

describe("applyFramePayload error frames", () => {
	it("routes backend error frames into appStore.error", () => {
		appStore.actions.clearError();

		applyFramePayload({
			error: {
				level: "error",
				error: "get websockets token: EAPI:Invalid nonce",
				caller: "websocket/live.go:134",
			},
		});

		expect(appStore.state.error).toEqual({
			level: "error",
			error: "get websockets token: EAPI:Invalid nonce",
			caller: "websocket/live.go:134",
		});
	});

	it("clears the overlay state", () => {
		appStore.actions.updateError({ message: "temporary" });
		appStore.actions.clearError();
		expect(appStore.state.error).toBeNull();
	});
});
