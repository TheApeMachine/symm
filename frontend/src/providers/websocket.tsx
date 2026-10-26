import { atoms, signals, updateClock, valueAtoms } from "#/collections/app";
import "#/collections/hub-projections";
import { ensureLongtailRecorder } from "#/collections/longtail";
import { syncEngineTelemetry } from "#/lib/engine-telemetry";
import { hubWsUrl } from "#/lib/hub";
import { bindHubWorker, sendFocus, sendRoute } from "#/lib/hub-socket";
import { parseValues } from "#/types/decoder";
import type { Value } from "#/types/value";
import { batch, type Store } from "@tanstack/store";
import { useEffect } from "react";

type SignalStore = Store<Record<string, Value>>;

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const advanceMarketTick = (value: Value) => {
	if (value.origin !== "workspace" || value.scope !== "aggregate") {
		return;
	}

	const tick = typeof value.tick === "bigint" ? Number(value.tick) : value.tick;
	atoms.tickCount.set(tick);
	updateClock(value.timestamp);
};

/*
The workspace drain can fan out the aggregate tick before later pipeline
stages append signal and grid layers. A second frame for the same tick may
carry fewer layers — keep the richer one so impulse map and kernel readers
do not lose live metrics.
*/
const keepExistingWorkspaceAggregate = (
	existing: Value | undefined,
	incoming: Value,
): boolean => {
	if (!existing || incoming.scope !== "aggregate") {
		return false;
	}
	if (existing.tick !== incoming.tick) {
		return false;
	}
	return incoming.index.length < existing.index.length;
};

const patchSignalStores = (pending: Map<SignalStore, Map<string, Value>>) => {
	for (const [store, scopes] of pending) {
		store.setState((prev) => {
			let next: Record<string, Value> | null = null;

			for (const [scope, frame] of scopes) {
				if (prev[scope] === frame) {
					continue;
				}

				if (
					store === signals.workspace &&
					scope === "aggregate" &&
					keepExistingWorkspaceAggregate(prev[scope], frame)
				) {
					continue;
				}

				if (next === null) {
					next = { ...prev };
				}
				next[scope] = frame;
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

			const store = signals[value.origin];

			if (!store) {
				continue;
			}

			let scopes = pending.get(store);
			if (!scopes) {
				scopes = new Map();
				pending.set(store, scopes);
			}
			scopes.set(value.scope, value);

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
