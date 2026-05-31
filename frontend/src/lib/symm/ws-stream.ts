import { useCallback } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket.js";

import { isConfidenceRow } from "#/components/symm/confidence-data-provider";
import { ConnectionStore } from "#/lib/symm/connection-store";
import {
	isAuditEvent,
	isDecisionTraceEvent,
	isEnginePulseEvent,
	isHeartbeatEvent,
	isHelloEvent,
	isPredictionFeedback,
	isTickEvent,
	isWalletPayload,
} from "#/lib/symm/events";
import { isLayoutDocument } from "#/lib/symm/layout-schema";
import { LayoutStore } from "#/lib/symm/layout-store";
import { StreamDataProvider } from "#/lib/symm/stream-data-provider";
import { useSymmTelemetryStores } from "#/lib/symm/telemetry-context";
import type { SymmTelemetryStores } from "#/lib/symm/telemetry-stores";
import { TickStore } from "#/lib/symm/tick-store";

const resolveSocketUrl = () => {
	if (typeof window === "undefined") {
		return null;
	}

	const custom = import.meta.env.VITE_SYMM_WS_URL?.trim();

	if (custom) {
		return custom;
	}

	return "ws://127.0.0.1:8765/ws";
};

const parseWirePayload = (raw: unknown): unknown | null => {
	if (typeof raw !== "string") {
		return null;
	}

	const trimmed = raw.trim();

	if (trimmed.length === 0) {
		return null;
	}

	return JSON.parse(trimmed) as unknown;
};

export const routePayload = (stores: SymmTelemetryStores, payload: unknown) => {
	if (isHelloEvent(payload)) {
		ConnectionStore.set(true);
		return;
	}

	if (isLayoutDocument(payload)) {
		LayoutStore.ingest(payload);
		return;
	}

	if (isTickEvent(payload)) {
		TickStore.ingest();
		return;
	}

	if (isHeartbeatEvent(payload)) {
		TickStore.ingestHeartbeat(payload);
		return;
	}

	if (isAuditEvent(payload)) {
		stores.audit.ingest(payload);
		return;
	}

	if (isDecisionTraceEvent(payload)) {
		stores.decisions.ingest(payload);
		return;
	}

	if (isPredictionFeedback(payload)) {
		stores.predictions.ingest(payload);
		return;
	}

	if (isConfidenceRow(payload)) {
		stores.confidence.ingest(payload);
		return;
	}

	if (isWalletPayload(payload)) {
		stores.wallet.ingest(payload);
		stores.trades.ingest(payload);
		stores.confidence.ingestSnapshot(payload.gauge_confidence);
		return;
	}

	if (isEnginePulseEvent(payload)) {
		stores.predictions.ingest(payload);
		return;
	}

	if (typeof payload === "object" && payload !== null) {
		const row = payload as Record<string, unknown>;

		if (typeof row.event === "string") {
			switch (row.event) {
				case "prediction":
				case "prediction_settled":
					stores.predictions.ingest(payload);
					StreamDataProvider.ingest(row.event, payload);
					return;
				case "field_row":
				case "field_snapshot":
				case "field_grid":
					stores.fluid.ingest(payload);
					return;
				case "candle_bar":
					stores.ohlc.ingest(payload);
					StreamDataProvider.ingest("candle_bar", payload);
					if (typeof row.symbol === "string" && typeof row.close === "number") {
						stores.trades.setMark(row.symbol, row.close);
					}
					return;
				case "mark":
					if (typeof row.symbol === "string" && typeof row.price === "number") {
						stores.trades.setMark(row.symbol, row.price);
					}
					return;
				case "wallet":
					stores.wallet.ingest(payload);
					stores.trades.ingest(payload);
					stores.confidence.ingestSnapshot(row.gauge_confidence);
					return;
				default:
					break;
			}
		}
	}

	stores.trades.ingest(payload);
	stores.ohlc.ingest(payload);
};

export const useSymmStream = () => {
	const stores = useSymmTelemetryStores();

	const onMessage = useCallback(
		(event: MessageEvent<string>) => {
			let payload: unknown | null;

			try {
				payload = parseWirePayload(event.data);
			} catch {
				ConnectionStore.set(false);
				return;
			}

			if (payload === null) {
				return;
			}

			routePayload(stores, payload);
		},
		[stores],
	);

	useWebSocket(resolveSocketUrl(), {
		shouldReconnect: () => true,
		onMessage,
		onClose: () => ConnectionStore.set(false),
		onError: () => ConnectionStore.set(false),
	});
};
