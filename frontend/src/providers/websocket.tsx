import { batch, type Store } from "@tanstack/store";
import { useEffect } from "react";
import {
	atoms,
	ensureSignalStore,
	signals,
	updateClock,
	valueAtoms,
} from "#/collections/app";
import "#/collections/hub-projections";
import { ensureLongtailRecorder } from "#/collections/longtail";
import { mergePositionBook } from "#/collections/positions";
import { applyS3PrefixCompacts } from "#/hub/apply-s3-compact";
import type { S3PrefixCompact } from "#/hub/s3-prefix-compact";
import { syncEngineTelemetry } from "#/lib/engine-telemetry";
import { hubWsUrl } from "#/lib/hub";
import { bindHubWorker, sendFocus, sendRoute } from "#/lib/hub-socket";
import {
	ingestHistoryClosedFrame,
	requestClosedPositionHistory,
	resetClosedHistoryRequest,
} from "#/lib/lifecycle";
import { valueFromWire, type WireValue } from "#/types/decoder";
import {
	mergeWorkspaceAggregate,
	type Value,
} from "#/types/value";

type SignalStore = Store<Record<string, Value>>;

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const frameScope = (value: Value): string => {
	if (
		value.origin === "workspace" &&
		value.role === "tick" &&
		value.scope === ""
	) {
		return "aggregate";
	}

	return value.scope || "aggregate";
};

const advanceMarketTick = (value: Value) => {
	if (value.origin !== "workspace") return;
	if (frameScope(value) !== "aggregate") return;

	const tick = typeof value.tick === "bigint" ? Number(value.tick) : value.tick;
	if (atoms.tickCount.get() !== tick) {
		atoms.tickCount.set(tick);
	}
	updateClock(value.timestamp);
};

/*
Put one frame at origin/scope. Same tick keeps prior layers so multiple
metric publishes for one symbol in one tick do not wipe each other; a new
tick replaces the slot. Grid/train frames are replaced wholesale each tick
so the Impulse Map tracks live brightness, not a historical peak.
*/
const putFrame = (
	store: SignalStore,
	scope: string,
	frame: Value,
) => {
	store.setState((prev) => {
		const prior = prev[scope];
		let next = frame;

		if (store === signals.positions) {
			return mergePositionBook(prev, frame);
		}

		if (prior && prior.tick === frame.tick) {
			next = mergeWorkspaceAggregate(prior, frame);
			if (next === prior) return prev;
		}

		if (prior === next) return prev;
		return { ...prev, [scope]: next };
	});
};

export const ingest = (values: Value[]) => {
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

			if (!value.origin) continue;

			const store = ensureSignalStore(value.origin);
			const scope = frameScope(value);
			putFrame(store, scope, value);
			advanceMarketTick(value);
		}
	});

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

			if (data.type === "VALUES") {
				const s3 = data.s3 as S3PrefixCompact[] | undefined;
				if (s3?.length) {
					applyS3PrefixCompacts(s3);
				}

				const wire = data.values as WireValue[];
				if (wire?.length) {
					ingest(wire.map(valueFromWire));
				}
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
