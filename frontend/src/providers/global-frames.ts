import { appStore } from "#/collections/app";

const DASHBOARD_FRAME_TYPES = new Set([
	"balances",
	"positions",
	"ohlc",
	"gauge",
	"regime",
	"fluid",
	"manifold",
	"prediction",
]);

export const statusSocketHandlers = {
	shouldReconnect: () => true,
	onOpen: () => appStore.actions.updateOnline(true),
	onClose: () => appStore.actions.updateOnline(false),
};

/*
applyGlobalFrame returns true when a frame is owned by the root WsFeed dashboard
pipeline so secondary route handlers can ignore it.
*/
export const applyGlobalFrame = (raw: Record<string, unknown>): boolean => {
	const frameType = raw.type;

	return typeof frameType === "string" && DASHBOARD_FRAME_TYPES.has(frameType);
};
