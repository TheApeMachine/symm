import { useEffect } from "react";
import { atoms, signals } from "#/collections/app";
import { ingestConfidence } from "#/collections/confidence";
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

export { sendCommand, sendRoute } from "#/lib/hub-socket";

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
			const next = value as { paper?: Balance; real?: Balance };
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
			const light = value as {
				n?: number;
				decisions?: Record<string, unknown>;
			} | null;
			store.setState(() => value);
			if (typeof light?.n === "number" && light.n > 0) {
				atoms.tickCount.set(light.n);
			}
			if (light?.decisions && typeof light.decisions === "object") {
				signals.decisions.setState((prev) => ({
					...prev,
					...light.decisions,
				}));
			}
			continue;
		}

		if (key === "positions") {
			const lots = (value ?? {}) as Record<string, Position>;
			if (mode === "snapshot") {
				store.setState(() => lots);
			} else {
				store.setState((prev) => ({ ...prev, ...lots }));
			}
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

			if (data.type === "ENVELOPE") {
				ingestEnvelope(data.payload as Record<string, unknown>);
				syncEngineTelemetry();
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
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			worker.postMessage({ type: "DISCONNECT" });
			worker.terminate();
			bindHubWorker(null);
		};
	}, []);

	return null;
};
