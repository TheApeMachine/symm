import { batch as storeBatch } from "@tanstack/react-store";
import { useEffect } from "react";
import {
	boundAtom,
	evictStaleSymbols,
	evictSymbol,
	focusAtom,
	onlineAtom,
	routeAtom,
} from "#/collections/app";
import { type Bound, readBindings } from "#/types/capnp/bindings";

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

/*
dispatchBindings lands the values that reached component ports in the running
program on the components they were wired to.
*/
function dispatchBindings(values: Bound[]) {
	if (values.length === 0) {
		return;
	}

	boundAtom.set((previous) => {
		const next = { ...previous };

		for (const bound of values) {
			const graph = { ...(next[bound.graph] ?? {}) };
			graph[bound.component] = {
				...(graph[bound.component] ?? {}),
				[bound.prop]: bound.value,
			};
			next[bound.graph] = graph;
		}

		return next;
	});
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
		`WS: ignoring a ${bytes}-byte frame that is not a bindings frame (further ones of this size will be counted, not logged):`,
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
					const values = readBindings(data.buffer);
					storeBatch(() => {
						dispatchBindings(values);
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
