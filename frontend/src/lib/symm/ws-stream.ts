import { useEffect } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket.js";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { CausalGraphDataProvider } from "#/components/symm/causal-graph-data-provider";
import { EnginePulseDataProvider } from "#/components/symm/engine-pulse-data-provider";
import { FluidDataProvider } from "#/components/symm/fluid-data-provider";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";
import { WalletDataProvider } from "#/components/symm/wallet-data-provider";
import { ConnectionStore } from "#/lib/symm/connection-store";
import { isHelloEvent } from "#/lib/symm/events";

const socketUrl = "ws://localhost:8765/ws";

const routePayload = (payload: unknown) => {
	if (isHelloEvent(payload)) {
		ConnectionStore.set(true);
		return;
	}

	if (typeof payload === "object" && payload !== null) {
		const row = payload as Record<string, unknown>;

		if (typeof row.event === "string") {
			switch (row.event) {
				case "engine_pulse":
					EnginePulseDataProvider.ingest(payload);
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

	WalletDataProvider.ingest(payload);
	TradesDataProvider.ingest(payload);
	CausalGraphDataProvider.ingest(payload);
	ConfidenceDataProvider.ingest(payload);
	OhlcDataProvider.ingest(payload);
};

export const useSymmStream = () => {
	const { lastMessage } = useWebSocket(
		typeof window === "undefined" ? null : socketUrl,
		{
			shouldReconnect: () => true,
		},
	);

	useEffect(() => {
		if (lastMessage === null) {
			return;
		}

		routePayload(JSON.parse(lastMessage.data));
	}, [lastMessage]);
};
