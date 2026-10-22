import { atoms, RingBuffer, signals } from "#/collections/app";
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
The worker parses wire frames and posts them up; this thread owns the stores
and atoms, so every write happens where React reads them.

Signal stores are mutated in place (ring.add never replaces the store's state
object), so selectors never fire. One counter per store: after a batch lands,
the state is replaced with a fresh object carrying the bump and the same ring
map, which is what triggers re-renders. The counter lives on the state itself
so a worker reload cannot re-issue a stale identity.
*/
const bumpStore = (store: {
	state: Record<string, unknown>;
	setState: (s: never) => void;
}) => {
	const revision = Number(store.state.revision ?? 0) + 1;
	store.setState({ ...store.state, revision } as never);
};

const ingest = (values: Value[]) => {
	for (const val of values) {
		const origin = val.origin;

		// Route by origin: atom or signal store.
		const atom = atoms[origin];
		const store = signals[origin];

		if (atom) {
			// Atom origins (balance, ...) carry their payload in one layer;
			// components read it via Value.lookup on the atom's current Value.
			atoms[origin].set(val);
		}

		if (store) {
			const symbol = val.scope;
			let ring = store.state[symbol] as RingBuffer<Value> | undefined;

			if (!ring) {
				ring = new RingBuffer<Value>(50);
				store.state[symbol] = ring;
			}

			ring.add(val);
			bumpStore(store);
		}

		/*
		The region ring and the token strip read from the cognition stores,
		not the training store: every train/regions layer on the value feeds
		them here, at ingest, the same for any origin that carries one.
		*/
		if (val.index.some((index) => index.includes("train/regions"))) {
			for (let i = 0; i < val.index.length; i++) {
				if (val.index[i].includes("train/regions")) {
					let payload: unknown;
					try {
						payload = JSON.parse(new TextDecoder().decode(val.layers[i]));
					} catch {
						continue;
					}
					ingestTrainRegions(Number(val.tick), val.index[i], payload);
				}
			}
		}

		/*
		The grammar feed rides origin "sequitur": one frame per symbol per
		training tick, decoded once and folded into the Sequitur store.
		The Sequitur tab renders it; the Longtail recorder reads the same
		store for the gate and policy rates.

		The cognition feed rides origin "cognition" with role "match":
		the live sentence and its policy hit, for the Cognitive tree and
		the forward-learning trie. One decoder per layer.
		*/
		if (origin === "sequitur" || origin === "cognition") {
			for (let i = 0; i < val.index.length; i++) {
				let payload: unknown;
				try {
					payload = JSON.parse(new TextDecoder().decode(val.layers[i]));
				} catch {
					continue;
				}

				if (origin === "sequitur") {
					ingestSequiturFrame(payload);
					continue;
				}

				/*
				The wire epoch is a uint64: it exceeds what a JS number
				holds exactly, and the store keys epochs as strings.
				*/
				if (
					val.role === "match" &&
					payload &&
					typeof payload === "object"
				) {
					(payload as Record<string, unknown>).epoch = String(
						val.epoch,
					);
				}

				ingestCognitionFrame(val.role, payload);
			}
		}

		/*
		The tick is a monotonic clock, not a per-frame payload. Frames that
		carry no tick (or a zero/unparseable one) must not reset the
		count — keep the last one seen.
		*/
		if (val.tick != null) {
			const tick = Number(val.tick);

			if (Number.isFinite(tick) && tick > 0) {
				atoms.tickCount.set(tick);
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
				console.log("values", values);
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
