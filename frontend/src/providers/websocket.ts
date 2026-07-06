import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { tickStore } from "#/collections/tick";
import type { Measurement } from "#/types/measurement";
import { parseMeasurements, bestCategory, isMeasurement } from "#/types/measurement";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

/*
CASCADE_SOURCES are produced by the decision ladder, not by individual
signals. The backend's Decision type generates manifold/resonance/causal
internally — they may appear as measurement sources, and we route them
to their dedicated stores for the x-ray and dashboard panels.
*/
const CASCADE_SOURCES = new Set(["manifold", "resonance", "causal"]);

/*
routeMeasurement distributes a single typed Measurement to the stores
that downstream components read from. The measurementsStore always
receives every measurement; additional stores receive derived data.
*/
const routeMeasurement = (measurement: Measurement) => {
	const source = measurement.source;
	const category = bestCategory(measurement.categories);

	if (source === "fluid" || source === "manifold") {
		manifoldStore.actions.updateFrame({
			...measurement,
			...measurement.metrics,
			category: category?.type ?? "",
		});
	}

	if (CASCADE_SOURCES.has(source)) {
		resonanceStore.actions.updateFrame({
			...measurement,
			...measurement.metrics,
			type: `resonance_${source}`,
			symbol: measurement.symbol,
			category: category?.type ?? "",
		});
	}

	if (source === "cognitive") {
		const reading = {
			scope: measurement.symbol,
			sequence: String(measurement.metrics.sequence ?? ""),
			regimePrefix: category?.type ?? "",
			regimeCohort: measurement.metrics.cohort ?? 0,
			ambiguous: (measurement.metrics.ambiguous ?? 0) > 0.5,
			sideline: (measurement.metrics.sideline ?? 0) > 0.5,
			entropyBits: measurement.metrics.entropyBits ?? 0,
			entropyThreshold: measurement.metrics.entropyThreshold ?? 0,
			classConfidence: category?.confidence ?? 0,
			contrastEvidence: measurement.metrics.contrastEvidence ?? 0,
			lookaheadScore: measurement.metrics.lookaheadScore ?? 0,
			lookaheadPaths: measurement.metrics.lookaheadPaths ?? 0,
			winnerClass: category?.type ?? "",
			updatedAt: Date.now(),
		};

		cognitiveStore.actions.updateFrame({
			readings: { [measurement.symbol]: reading },
		});
	}
};

/*
routeBatch processes a complete []Measurement frame from the WebSocket.
It first ingests the full batch into measurementsStore, then routes
individual measurements to their specialized stores.
*/
export const routeBatch = (batch: Measurement[]) => {
	if (batch.length === 0) {
		return;
	}

	measurementsStore.actions.ingestBatch(batch);
	appStore.actions.observeSources(measurementsStore.state.sources);

	const symbols = new Set<string>();

	for (const measurement of batch) {
		routeMeasurement(measurement);

		if (measurement.symbol.includes("/")) {
			symbols.add(measurement.symbol);
		}
	}

	if (symbols.size > 0) {
		instrumentsStore.actions.updateFrame({
			pairs: [...symbols].map((symbol) => ({ symbol })),
		});
	}

	tickStore.actions.updateFrame({
		count: measurementsStore.state.tick,
		phase: "measure",
		measurements: batch.length,
	});

	decisionStore.actions.observeTick(measurementsStore.state.tick);
};

/*
isAction checks if an item looks like a logic.Action from the decision ladder.
*/
const isAction = (value: unknown): boolean =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof (value as Record<string, unknown>).verdict === "string" &&
	typeof (value as Record<string, unknown>).score === "number";

/*
isBalanceData checks if an item looks like a kraken.BalanceData.
*/
const isBalanceData = (value: unknown): boolean =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof (value as Record<string, unknown>).asset === "string" &&
	typeof (value as Record<string, unknown>).balance === "number";

/*
isExecutionData checks if an item looks like a kraken.ExecutionData.
*/
const isExecutionData = (value: unknown): boolean =>
	value !== null &&
	typeof value === "object" &&
	!Array.isArray(value) &&
	typeof (value as Record<string, unknown>).exec_type === "string" &&
	typeof (value as Record<string, unknown>).order_id === "string";

/*
routeMessage handles data from the WebSocket. The backend sends data
as-is — no envelope. It can be:
  - []Measurement (array with source/symbol/categories)
  - []Action (array with verdict/score/symbol)
  - []BalanceData (array with asset/balance/available)
  - []ExecutionData (array with exec_type/order_id/symbol)

We detect the shape from the first element and route accordingly.
*/
export const routeMessage = (data: unknown) => {
	if (!Array.isArray(data) || data.length === 0) {
		return;
	}

	const first = data[0];

	if (isMeasurement(first)) {
		routeBatch(parseMeasurements(data));
		return;
	}

	if (isAction(first)) {
		for (const action of data) {
			decisionStore.actions.updateFrame(
				action as Record<string, unknown>,
			);
		}

		return;
	}

	if (isBalanceData(first)) {
		balancesStore.actions.updateFrame({
			rows: data,
			assets: data as Record<string, unknown>[],
		});

		return;
	}

	if (isExecutionData(first)) {
		executionsStore.actions.updateFrames(
			data as Record<string, unknown>[],
		);
	}
};

export const WsFeed = () => {
	const { updateOnline, updateError } = appStore.actions;

	useEffect(() => {
		let closedByUnmount = false;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let attempt = 0;
		let socket: WebSocket | null = null;

		const scheduleReconnect = () => {
			if (closedByUnmount || reconnectTimer !== null) {
				return;
			}

			const delay = Math.min(
				RECONNECT_MAX_MS,
				RECONNECT_BASE_MS * 2 ** attempt,
			);
			attempt += 1;
			reconnectTimer = setTimeout(() => {
				reconnectTimer = null;
				connect();
			}, delay);
		};

		const connect = () => {
			const currentSocket = new WebSocket(socketUrl);
			socket = currentSocket;

			currentSocket.addEventListener("open", () => {
				if (closedByUnmount || socket !== currentSocket) {
					currentSocket.close();
					return;
				}

				attempt = 0;
				updateOnline(true);
			});

			currentSocket.addEventListener("close", () => {
				if (closedByUnmount || socket !== currentSocket) {
					return;
				}

				updateOnline(false);
				scheduleReconnect();
			});

			currentSocket.addEventListener("error", () => {
				if (socket !== currentSocket) {
					return;
				}
				if (currentSocket.readyState === WebSocket.OPEN) {
					currentSocket.close();
				}
			});

			currentSocket.addEventListener("message", (event) => {
				if (socket !== currentSocket) {
					return;
				}
				try {
					routeMessage(JSON.parse(String(event.data)));
				} catch (err) {
					console.error(err);
					updateError({ err: err });
				}
			});
		};

		connect();

		return () => {
			closedByUnmount = true;

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
			}

			if (socket?.readyState === WebSocket.OPEN) {
				socket.close();
			}
		};
	}, [updateOnline, updateError]);

	return null;
};
