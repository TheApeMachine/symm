import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { appStore } from "#/collections/app";
import { balanceStore } from "#/collections/balance";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

export const WsFeed = () => {
	const { updateOnline } = appStore.actions;

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => updateOnline(true),
		onClose: () => updateOnline(false),
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				switch (raw.type) {
					case "balances":
						balanceStore.setState((prev) => ({ ...prev, ...raw }));
						break;
					case "ohlc":
						appStore.state.candleUpdater?.(raw);
						break;
					case "gauge": {
						const source = typeof raw.source === "string" ? raw.source : "";

						if (source !== "") {
							appStore.state.gaugeUpdaters[source]?.(raw);
						}

						appStore.state.confidenceHeatmapUpdater?.(raw);
						appStore.state.surpriseHeatmapUpdater?.(raw);
						break;
					}
					case "regime":
						appStore.state.regimeUpdater?.(raw);
						break;
					case "fluid":
						appStore.state.fluidUpdater?.(raw);
						break;
					case "manifold":
						appStore.state.manifoldUpdater?.(raw);
						break;
					case "prediction":
						appStore.state.predictionUpdater?.(raw);
						break;
					default:
						break;
				}
			} catch (error) {
				console.error("websocket frame parse failed", error, event.data);
			}
		},
	});

	return null;
};
