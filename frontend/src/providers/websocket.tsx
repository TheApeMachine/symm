import { atoms, signals, updateClock } from "#/collections/app";
import {
	ingestCognitionFrame,
	ingestTrainRegions,
} from "#/collections/cognition";
import { ingestSequiturFrame } from "#/collections/sequitur";
import { hubWsUrl } from "#/lib/hub";
import { parseValues } from "#/types/decoder";
import type { Value } from "#/types/value";
import { useEffect } from "react";

/*
ingest routes one decoded batch. An origin with an atom (balance, ...) sets
it; an origin with a signal store takes the frame under its scope. The tick
atom and clock advance on every frame that carries a tick, and the cognition,
sequitur and training-region feeds are folded into their own stores here —
the same for any origin that carries their layers.
*/
const ingest = (values: Value[]) => {
	;(window as any).__kilo_probe = (window as any).__kilo_probe || {
		count: 0,
		origins: {} as Record<string, number>,
		scopes: {} as Record<string, number>,
		missed: [] as any[],
		stored: 0,
		last: null as any,
	};
	const probe = (window as any).__kilo_probe;
	probe.count++;
	for (const value of values) {
		probe.origins[value.origin] = (probe.origins[value.origin] || 0) + 1;
		probe.scopes[value.scope] = (probe.scopes[value.scope] || 0) + 1;
		if (probe.count < 5 || probe.last === null) {
			probe.last = {
				origin: value.origin,
				scope: value.scope,
				prefix: value.prefix,
				idxLen: value.index.length,
				idx0: value.index[0],
				idx1: value.index[1],
			};
		}
	}
	for (const value of values) {
		/*
		Only origins the dashboard binds to have a store; a frame from any
		other origin (kraken acks, desk lifecycle) is telemetry the socket
		delivers and ingest ignores rather than routes into nothing.
		*/
		const store = signals[value.origin];
		if (!store) {
			probe.missed = (probe.missed || []);
			if (probe.missed.length < 5) probe.missed.push({ origin: value.origin, scope: value.scope });
			continue;
		}
		probe.stored = (probe.stored || 0) + 1;

		store.setState((prev) => ({
			...prev,
			[value.scope]: value,
		}));

		/*
		train/regions layers feed the region ring and the token strip at
		ingest, whatever origin carried them.
		*/
		if (value.index.some((index) => index.includes("train/regions"))) {
			for (let i = 0; i < value.index.length; i++) {
				if (!value.index[i].includes("train/regions")) continue;
				let payload: unknown;
				try {
					payload = JSON.parse(new TextDecoder().decode(value.layers[i]));
				} catch {
					continue;
				}
				ingestTrainRegions(Number(value.tick), value.index[i], payload);
			}
		}

		/*
		The grammar feed rides origin "sequitur"; the cognition feed rides
		origin "cognition" with its role. One decoder per layer. The wire
		epoch is a uint64 beyond exact JS numbers: the store keys epochs
		as strings.
		*/
		if (value.origin === "sequitur" || value.origin === "cognition") {
			for (let i = 0; i < value.index.length; i++) {
				let payload: unknown;
				try {
					payload = JSON.parse(new TextDecoder().decode(value.layers[i]));
				} catch {
					continue;
				}

				if (value.origin === "sequitur") {
					ingestSequiturFrame(payload);
					continue;
				}

				if (value.role === "match" && payload && typeof payload === "object") {
					(payload as Record<string, unknown>).epoch = String(value.epoch);
				}

				ingestCognitionFrame(value.role, payload);
			}
		}

		/*
		The tick is a monotonic clock, not a per-frame payload. Frames that
		carry no tick (or a zero/unparseable one) must not reset the count —
		keep the last one seen.
		*/
		if (value.tick != null) {
			const tick = Number(value.tick);

			if (Number.isFinite(tick) && tick > 0) {
				atoms.tickCount.set(tick);
				updateClock(value.timestamp);
			}
		}
	}
};

/*
The hub multiplexes everything over one socket: each message is one serialized
*data.Value, routed here by its layer index. Commands go back on the same socket.
*/
let wsWorker: Worker | null = null;

const toBackendRoute = (route: string) => {
	// The learning surface hosts the Impulse Map and Position Health inspector,
	// which require focused signal streams (CVD, Hawkes, Depthflow, etc.) to light up
	// the SOM grid confluence regions.
	return route === "learning" ? "dashboard" : route;
};

export const sendRoute = (route: string) => {
	wsWorker?.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
};

/*
sendCommand sends one JSON command the hub answers on this socket. With no
socket the command cannot be sent, which is an error, not a no-op.
*/
export const sendCommand = (command: Record<string, unknown>) => {
	if (wsWorker === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}

	wsWorker.postMessage({ type: "COMMAND", command });
};

export const WsFeed = () => {
	useEffect(() => {
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

		worker.postMessage({
			type: "ROUTE",
			route: toBackendRoute(atoms.route.get()),
		});

		worker.postMessage({ type: "CONNECT", url });
		wsWorker = worker;

		const unsubscribeFocus = atoms.focus.subscribe((symbol: string) => {
			worker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = atoms.route.subscribe((route: string) => {
			worker.postMessage({ type: "ROUTE", route: toBackendRoute(route) });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			wsWorker = null;
		};
	}, []);

	return null;
};
