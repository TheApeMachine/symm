import { useEffect } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket.js";

import {
	ConfidenceDataProvider,
	isConfidenceRow,
} from "#/components/symm/confidence-data-provider";
import { FluidDataProvider } from "#/components/symm/fluid-data-provider";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";
import { WalletDataProvider } from "#/components/symm/wallet-data-provider";
import { ConnectionStore } from "#/lib/symm/connection-store";
import {
	isEnginePulseEvent,
	isHelloEvent,
	isPredictionFeedback,
	isWalletPayload,
} from "#/lib/symm/events";

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

const routePayload = (payload: unknown) => {
	if (isHelloEvent(payload)) {
		ConnectionStore.set(true);
		return;
	}

	if (isPredictionFeedback(payload)) {
		PredictionsDataProvider.ingest(payload);
		return;
	}

	if (isConfidenceRow(payload)) {
		ConfidenceDataProvider.ingest(payload);
		return;
	}

	if (isWalletPayload(payload)) {
		WalletDataProvider.ingest(payload);
		TradesDataProvider.ingest(payload);
		return;
	}

	if (isEnginePulseEvent(payload)) {
		PredictionsDataProvider.ingest(payload);
		return;
	}

	if (typeof payload === "object" && payload !== null) {
		const row = payload as Record<string, unknown>;

		if (typeof row.event === "string") {
			switch (row.event) {
				case "prediction":
					PredictionsDataProvider.ingest(payload);
					return;
				case "field_row":
				case "field_snapshot":
				case "field_grid":
					FluidDataProvider.ingest(payload);
					return;
				case "candle_bar":
					OhlcDataProvider.ingest(payload);
					return;
				default:
					break;
			}
		}
	}

	TradesDataProvider.ingest(payload);
	OhlcDataProvider.ingest(payload);
};

export const useSymmStream = () => {
	const { lastMessage } = useWebSocket(resolveSocketUrl(), {
		shouldReconnect: () => true,
	});

	useEffect(() => {
		if (lastMessage === null) {
			return;
		}

		let payload: unknown | null;

		try {
			payload = parseWirePayload(lastMessage.data);
		} catch {
			ConnectionStore.set(false);
			return;
		}

		if (payload === null) {
			return;
		}

		routePayload(payload);
	}, [lastMessage]);
};
