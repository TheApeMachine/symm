import { useEffect } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket.js";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { CausalGraphDataProvider } from "#/components/symm/causal-graph-data-provider";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";

const socketUrl = "ws://localhost:8765/ws";

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

		const payload = JSON.parse(lastMessage.data);

		CausalGraphDataProvider.ingest(payload);
		ConfidenceDataProvider.ingest(payload);
		OhlcDataProvider.ingest(payload);
	}, [lastMessage]);
};
