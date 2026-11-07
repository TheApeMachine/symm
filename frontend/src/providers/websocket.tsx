import { useEffect } from "react";
import { atoms, signals } from "#/collections/app";
import {
	decisionBook,
	decisionStoreKey,
	isBook,
} from "#/collections/book";
import { ingestConfidence } from "#/collections/confidence";
import type { Decision } from "#/collections/envelope";
import {
	ensureLongtailRecorder,
	requestLongtailSources,
	resetLongtailSourcesRequest,
} from "#/collections/longtail";
import type { Balance, Position } from "#/collections/types";
import {
	applyClosedEnvelope,
	applyPathsEnvelope,
} from "#/hub/apply-s3-compact";
import { syncEngineTelemetry } from "#/lib/engine-telemetry";
import { hubWsUrl } from "#/lib/hub";
import { bindHubWorker, sendFocus, sendRoute } from "#/lib/hub-socket";
import {
	requestClosedPositionHistory,
	resetClosedHistoryRequest,
} from "#/lib/lifecycle";
import { syncOpenPositionCount } from "#/lib/open-position-count";

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const asPosition = (raw: unknown): Position | null => {
	if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
		return null;
	}

	const row = raw as Position;
	if (typeof row.id !== "string" || row.id === "") {
		return null;
	}

	if (isBook(row.book)) {
		return row;
	}

	return row;
};

/*
positionsFromWire accepts either a lot map keyed by id or a bare array from
tier Hello, and keeps each lot under its id so virtual/paper/real can coexist.
*/
const positionsFromWire = (value: unknown): Record<string, Position> => {
	const lots: Record<string, Position> = {};

	const put = (raw: unknown) => {
		const position = asPosition(raw);
		if (!position) return;
		lots[position.id] = position;
	};

	if (Array.isArray(value)) {
		for (const row of value) {
			put(row);
		}
		return lots;
	}

	if (value && typeof value === "object") {
		for (const row of Object.values(value as Record<string, unknown>)) {
			put(row);
		}
	}

	return lots;
};

const decisionsFromWire = (
	value: unknown,
): Record<string, Decision> => {
	const next: Record<string, Decision> = {};

	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return next;
	}

	for (const raw of Object.values(value as Record<string, unknown>)) {
		if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;

		const decision = raw as Decision;
		if (!decision.symbol || !decision.action) continue;

		const book = decisionBook(decision);
		next[decisionStoreKey(decision)] = {
			...decision,
			tier: book,
		};
	}

	return next;
};

const mergeEnvelope = (
	into: Record<string, unknown>,
	payload: Record<string, unknown>,
) => {
	for (const [key, value] of Object.entries(payload)) {
		if (key === "positions_mode") {
			into.positions_mode = value;
			continue;
		}

		if (
			(key === "positions" ||
				key === "balances" ||
				key === "decisions" ||
				key === "paths" ||
				key === "closed") &&
			value &&
			typeof value === "object" &&
			!Array.isArray(value) &&
			into[key] &&
			typeof into[key] === "object" &&
			!Array.isArray(into[key])
		) {
			into[key] = {
				...(into[key] as Record<string, unknown>),
				...(value as Record<string, unknown>),
			};
			continue;
		}

		into[key] = value;
	}
};

const TELEMETRY_KEYS = new Set([
	"decisions",
	"confidence",
	"positions",
	"paths",
	"closed",
]);

const ingestEnvelope = (payload: Record<string, unknown>) => {
	const mode =
		typeof payload.positions_mode === "string"
			? payload.positions_mode
			: undefined;

	for (const [key, value] of Object.entries(payload)) {
		if (key === "positions_mode") {
			continue;
		}

		if (key === "balance") {
			atoms.balance.set(value as Balance);
			continue;
		}

		if (key === "balances" && value && typeof value === "object") {
			const next = value as {
				virtual?: Balance;
				paper?: Balance;
				real?: Balance;
			};
			atoms.balances.set({
				...atoms.balances.get(),
				...next,
			});
			continue;
		}

		if (key === "confidence") {
			ingestConfidence(value);
			continue;
		}

		if (key === "paths") {
			applyPathsEnvelope(value);
			continue;
		}

		if (key === "closed") {
			applyClosedEnvelope(value);
			continue;
		}

		const store = signals[key];
		if (!store) continue;

		if (key === "tick") {
			const light = value as { n?: number } | null;
			store.setState(() => value);
			if (typeof light?.n === "number" && light.n > 0) {
				atoms.tickCount.set(light.n);
			}
			continue;
		}

		if (key === "decisions") {
			const lots = decisionsFromWire(value);
			store.setState((prev) => ({ ...prev, ...lots }));
			continue;
		}

		if (key === "positions") {
			const lots = positionsFromWire(value);
			if (mode === "snapshot") {
				store.setState(() => lots);
			} else {
				store.setState((prev) => ({ ...prev, ...lots }));
			}
			syncOpenPositionCount();
			continue;
		}

		if (value && typeof value === "object" && !Array.isArray(value)) {
			store.setState((prev) => ({
				...prev,
				...(value as Record<string, unknown>),
			}));
			continue;
		}

		store.setState(() => value);
	}
};

const needsTelemetry = (payload: Record<string, unknown>) => {
	for (const key of Object.keys(payload)) {
		if (TELEMETRY_KEYS.has(key)) return true;
	}
	return false;
};

export const WsFeed = () => {
	useEffect(() => {
		ensureLongtailRecorder();

		const url = `${hubWsUrl()}/ws`;
		const worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});

		let pending: Record<string, unknown> | null = null;
		let raf = 0;

		const flush = () => {
			raf = 0;
			const payload = pending;
			pending = null;
			if (!payload) return;

			ingestEnvelope(payload);
			if (needsTelemetry(payload)) {
				syncEngineTelemetry();
			}
		};

		const queueEnvelope = (payload: Record<string, unknown>) => {
			if (!pending) {
				pending = { ...payload };
			} else {
				mergeEnvelope(pending, payload);
			}

			if (raf === 0) {
				raf = requestAnimationFrame(flush);
			}
		};

		worker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "ENVELOPE") {
				queueEnvelope(data.payload as Record<string, unknown>);
				return;
			}

			if (data.type === "COMMAND_FAILED") {
				console.error("hub command not sent:", data.command, data.error);
				return;
			}

			if (data.type === "STATUS") {
				if (data.status === "OFFLINE") {
					atoms.online.set("OFFLINE");
				}

				if (data.status === "ONLINE") {
					atoms.online.set("ONLINE");
					resetClosedHistoryRequest();
					resetLongtailSourcesRequest();
					requestClosedPositionHistory();
					requestLongtailSources();
				}

				if (data.status === "CONNECTING") {
					atoms.online.set("CONNECTING");
				}

				return;
			}

			if (data.type === "ERROR") {
				console.error("WS error:", data.error);
				return;
			}
		});

		bindHubWorker(worker);

		sendRoute(atoms.route.get());
		worker.postMessage({ type: "CONNECT", url });

		const unsubscribeFocus = atoms.focus.subscribe((symbol: string) => {
			sendFocus(symbol);
		});

		const unsubscribeRoute = atoms.route.subscribe((route: string) => {
			sendRoute(route);
		});

		return () => {
			if (raf !== 0) {
				cancelAnimationFrame(raf);
				flush();
			}

			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			bindHubWorker(null);
		};
	}, []);

	return null;
};
