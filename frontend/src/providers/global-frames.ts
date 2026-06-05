import { wsDispatchRef } from "#/providers/ws-dispatch";
import type { ActionVerdict } from "#/providers/ws-status";

/*
Every route opens its own websocket but they all share one status provider (the
header's wallet + online indicator, the positions and action panels). The frames
that feed that shared state — wallet, positions, decision cards — and the
connection status must therefore be handled identically no matter which page
opened the socket; otherwise a route that only cares about its own chart (e.g.
/decisions and its decision_tree) leaves the header stuck at its defaults
(€0.00 / Offline). These two helpers are that shared handling.
*/

// statusSocketHandlers wire a react-use-websocket connection's lifecycle to the
// shared online indicator. Spread into every route's useWebSocket config.
export const statusSocketHandlers = {
	shouldReconnect: () => true,
	onOpen: () => wsDispatchRef.current?.setOnline(true),
	onClose: () => wsDispatchRef.current?.setOnline(false),
	onError: () => wsDispatchRef.current?.setOnline(false),
};

// applyGlobalFrame dispatches the cross-route frames (wallet, positions, decision
// cards) to the shared status provider and returns true when it consumed the
// frame. Call it first in every route's onMessage; a false return means the frame
// is route-specific (gauges, decision_tree, …) and the caller should handle it.
export const applyGlobalFrame = (raw: Record<string, unknown>): boolean => {
	const dispatch = wsDispatchRef.current;

	if (!dispatch) {
		return false;
	}

	if (raw.event === "wallet") {
		dispatch.setWallet((raw.balance as number) ?? 0);
		return true;
	}

	if (raw.event === "equity") {
		dispatch.setEquity(
			(raw.exit_balance as number) ?? 0,
			(raw.capital_base as number) ?? 0,
		);
		return true;
	}

	if (raw.event === "mark") {
		dispatch.setMark(raw.symbol as string, (raw.price as number) ?? 0);
		return true;
	}

	if (raw.event === "positions") {
		const rows = (raw.positions as Record<string, unknown>[]) ?? [];
		dispatch.setPositions(
			rows.map((row) => ({
				symbol: row.symbol as string,
				qty: row.qty as number,
				avgEntry: row.avg_entry as number,
			})),
		);
		return true;
	}

	if (raw.event === "decision") {
		dispatch.pushAction({
			type: raw.type as string,
			symbol: raw.symbol as string,
			ts: Date.now(),
			verdict: (raw.verdict as ActionVerdict) ?? "rejected",
			reason: (raw.reason as string) ?? "",
		});
		return true;
	}

	return false;
};
