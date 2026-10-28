import { batch, type Store } from "@tanstack/store";
import { useEffect } from "react";
import { atoms, signals, updateClock, valueAtoms } from "#/collections/app";
import "#/collections/hub-projections";
import { ensureLongtailRecorder } from "#/collections/longtail";
import { mergePositionBook } from "#/collections/positions";
import { syncEngineTelemetry } from "#/lib/engine-telemetry";
import { hubWsUrl } from "#/lib/hub";
import { bindHubWorker, sendFocus, sendRoute } from "#/lib/hub-socket";
import {
	ingestHistoryClosedFrame,
	requestClosedPositionHistory,
	resetClosedHistoryRequest,
} from "#/lib/lifecycle";
import { parseValues } from "#/types/decoder";
import {
	mergeWorkspaceAggregate,
	type Value,
	workspaceAggregateEqual,
} from "#/types/value";

type SignalStore = Store<Record<string, Value>>;

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const workspaceAggregateScope = (value: Value): string => {
	if (
		value.origin === "workspace" &&
		value.role === "tick" &&
		value.scope === ""
	) {
		return "aggregate";
	}
	return value.scope;
};

const advanceMarketTick = (value: Value) => {
	if (value.origin !== "workspace") {
		return;
	}
	if (workspaceAggregateScope(value) !== "aggregate") {
		return;
	}

	const tick = typeof value.tick === "bigint" ? Number(value.tick) : value.tick;
	if (atoms.tickCount.get() !== tick) {
		atoms.tickCount.set(tick);
	}
	updateClock(value.timestamp);
};

const patchSignalStores = (pending: Map<SignalStore, Map<string, Value>>) => {
	for (const [store, scopes] of pending) {
		if (store === signals.positions) {
			store.setState((prev) => {
				let next = prev;
				for (const frame of scopes.values()) {
					next = mergePositionBook(next, frame);
				}
				return next;
			});
			continue;
		}

		store.setState((prev) => {
			let next: Record<string, Value> | null = null;

			for (const [scope, frame] of scopes) {
				if (prev[scope] === frame) {
					continue;
				}

				let nextFrame = frame;
				if (store === signals.workspace && scope === "aggregate") {
					nextFrame = mergeWorkspaceAggregate(prev[scope], frame);
					if (workspaceAggregateEqual(prev[scope], nextFrame)) {
						continue;
					}
				}

				if (prev[scope] === nextFrame) {
					continue;
				}

				if (next === null) {
					next = { ...prev };
				}
				next[scope] = nextFrame;
			}

			return next ?? prev;
		});
	}
};

export const ingest = (values: Value[]) => {
	const pending = new Map<SignalStore, Map<string, Value>>();

	batch(() => {
		for (const value of values) {
			const atom = valueAtoms[value.origin];

			if (atom) {
				atom.set(value);
				continue;
			}

			if (
				(value.origin === "position" || value.origin === "virtual") &&
				value.role === "closed"
			) {
				ingestHistoryClosedFrame(value);
				continue;
			}

			const store = signals[value.origin];

			if (!store) {
				continue;
			}

			const scope = workspaceAggregateScope(value);

			let scopes = pending.get(store);
			if (!scopes) {
				scopes = new Map();
				pending.set(store, scopes);
			}
			scopes.set(scope, value);

			advanceMarketTick(value);
		}

		patchSignalStores(pending);
	});

	/*
	Projections run when the batch flushes, so the engine readout has to
	read them after that, once matches and the tick are both current.
	*/
	syncEngineTelemetry();
};

export const WsFeed = () => {
	useEffect(() => {
		ensureLongtailRecorder();

		const url = `${hubWsUrl()}/ws`;
		const worker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});

		worker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "FRAMES") {
				const values = parseValues(new Uint8Array(data.buffer));
				if (values) ingest(values);
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
					requestClosedPositionHistory();
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
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			bindHubWorker(null);
		};
	}, []);

	return null;
};
