import { atoms, signals, updateClock, valueAtoms } from "#/collections/app";
import "#/collections/hub-projections";
import { ensureLongtailRecorder } from "#/collections/longtail";
import { syncEngineTelemetry } from "#/lib/engine-telemetry";
import { hubWsUrl } from "#/lib/hub";
import { bindHubWorker, sendFocus, sendRoute } from "#/lib/hub-socket";
import { parseValues } from "#/types/decoder";
import type { Value } from "#/types/value";
import { batch } from "@tanstack/store";
import { useEffect } from "react";

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const advanceMarketTick = (value: Value) => {
	if (value.origin !== "workspace" || value.scope !== "aggregate") {
		return;
	}

	atoms.tickCount.set(value.tick);
	updateClock(value.timestamp);
};

export const ingest = (values: Value[]) => {
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

			store.setState((prev) => ({
				...prev,
				[value.scope]: value,
			}));

			advanceMarketTick(value);
		}
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
