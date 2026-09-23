import { batch as storeBatch } from "@tanstack/react-store";
import { useEffect } from "react";
import {
	evictStaleSymbols,
	evictSymbol,
	focusAtom,
	onlineAtom,
	RingBuffer,
	routeAtom,
	signals,
	symbolsAtom,
	tickCountAtom,
	updateClock,
	updateEquity,
} from "#/collections/app";
import {
	readWireMeasurement,
	type WireMeasurement,
} from "#/types/capnp/measurement";

let globalWsWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;

export const sendPositionExit = (symbol: string) => {
	globalWsWorker?.postMessage({
		type: "POSITION_EXIT",
		symbol,
	});
};

export const sendRoute = (route: string) => {
	globalWsWorker?.postMessage({
		type: "ROUTE",
		route,
	});
};

const defaultWsUrl = () => {
	if (typeof window === "undefined") return "ws://127.0.0.1:8765/ws";
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host = window.location.hostname || "127.0.0.1";
	return `${protocol}//${host}:8765/ws`;
};

function dispatchMeasurement(row: WireMeasurement) {
	const symbolStr = row.symbol || "";
	const source = row.source || "";
	if (symbolStr && !symbolsAtom.get().includes(symbolStr)) {
		symbolsAtom.set(Array.from(new Set([...symbolsAtom.get(), symbolStr])));
	}

	if (row.at > 0n) {
		updateClock(row.at);
	}
	if (row.tick > 0n) {
		tickCountAtom.set(Number(row.tick));
	}

	if (source === "manifold") {
		signals.manifold.setState((prev: any) => ({
			...prev,
			[symbolStr]: row,
		}));
		signals.manifold.setState((prev: any) => ({ ...prev }));
		return;
	}

	if (source === "decision" || source === "strategy") {
		const meta = row.metadata || {};
		const action = String(meta["action"] || "wait");
		const reason = String(meta["reason"] || "attractor transition basin");
		const confidence =
			meta["confidence"] !== undefined ? Number(meta["confidence"]) : row.snr;

		signals.strategy.setState(() => [
			{
				decisions: [
					{
						id: `dec-${symbolStr}`,
						symbol: symbolStr,
						action,
						confidence,
						reason,
					},
				],
			},
		]);
		return;
	}

	if (source === "equity" || source === "balance") {
		const meta = row.metadata || {};
		const cashVal = String(meta["cash"] || "");
		const unrealizedVal = String(meta["unrealized"] || "");
		const equityVal = String(meta["equity"] || "");
		updateEquity(cashVal, unrealizedVal, equityVal);
		return;
	}

	const signalStore = signals[source as keyof typeof signals];
	if (!signalStore) {
		return;
	}

	let ring = signalStore.state[symbolStr];

	if (!ring) {
		ring = new RingBuffer<WireMeasurement>(50);
		signalStore.state[symbolStr] = ring;
	}

	ring.add(row);

	if (source === "training") {
		signalStore.state[""] = ring;
		signalStore.state["learner"] = ring;
	}

	signals[source as keyof typeof signals]?.setState((prev: any) => ({
		...prev,
	}));
}

/*
What has already been said about frames this socket could not read. Keyed by
size, because a peer sending the wrong thing sends the same wrong thing.
*/
const undecodable = new Map<number, number>();

const reportUndecodableFrame = (bytes: number, err: unknown) => {
	const seen = (undecodable.get(bytes) ?? 0) + 1;
	undecodable.set(bytes, seen);

	if (seen > 1) {
		return;
	}

	console.error(
		`WS: ignoring a ${bytes}-byte frame that is not a measurement (further ones of this size will be counted, not logged):`,
		err,
	);
};

/*
undecodableFrameCounts reports what this socket has been unable to read, so a
surface or a test can show that frames are arriving and being dropped rather
than leaving it to whoever happens to have the console open.
*/
export const undecodableFrameCounts = (): ReadonlyMap<number, number> =>
	new Map(undecodable);

export const WsFeed = () => {
	useEffect(() => {
		const wsWorker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});
		globalWsWorker = wsWorker;

		const wsUrl = import.meta.env.VITE_SYMM_WS_URL || defaultWsUrl();

		wsWorker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "STATUS") {
				onlineAtom.set(data.status);
				if (data.status === "ONLINE") {
					wsWorker.postMessage({ type: "FOCUS", symbol: focusAtom.get() });
				}
				return;
			}

			if (data.type === "UNSUBSCRIBE" && typeof data.symbol === "string") {
				evictSymbol(data.symbol);
				return;
			}

			if (data.type === "ERROR") {
				console.error("WS error:", data.error);
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
					const row = readWireMeasurement(data.buffer);
					storeBatch(() => {
						dispatchMeasurement(row);
					});
				} catch (err) {
					// A frame that cannot be read is reported once per shape
					// rather than on every arrival: a peer sending something
					// else on this socket would otherwise bury the console
					// under one identical stack every few seconds, which is
					// how this went unnoticed.
					reportUndecodableFrame(data.buffer.byteLength, err);
				}
			}
		});

		wsWorker.postMessage({ type: "ROUTE", route: routeAtom.get() });
		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });

		const unsubscribeFocus = focusAtom.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		const unsubscribeRoute = routeAtom.subscribe((route: string) => {
			wsWorker.postMessage({ type: "ROUTE", route });
		});

		const evictionInterval = setInterval(() => {
			evictStaleSymbols();
		}, 60_000);

		return () => {
			clearInterval(evictionInterval);
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			globalWsWorker = null;
		};
	}, []);

	return null;
};
