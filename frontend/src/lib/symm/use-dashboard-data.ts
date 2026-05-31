import { useSyncExternalStore } from "react";

import { ConnectionStore } from "#/lib/symm/connection-store";
import { useSymmTelemetryStores } from "#/lib/symm/telemetry-context";
import { TickStore } from "#/lib/symm/tick-store";

export const useSymmConnected = () =>
	useSyncExternalStore(
		ConnectionStore.subscribe,
		() => ConnectionStore.snapshot(),
		() => false,
	);

export const useSymmTick = () =>
	useSyncExternalStore(TickStore.subscribe, TickStore.snapshot, () => 0);

export const useSymmTelemetryStatus = () =>
	useSyncExternalStore(
		TickStore.subscribe,
		TickStore.statusSnapshot,
		TickStore.statusSnapshot,
	);

export const useSymmEnginePulse = () => {
	const stores = useSymmTelemetryStores();

	return useSyncExternalStore(
		stores.predictions.subscribe,
		stores.predictions.snapshot,
		() => undefined,
	);
};

export const useSymmWallet = () => {
	const stores = useSymmTelemetryStores();

	return useSyncExternalStore(
		stores.wallet.subscribe,
		stores.wallet.snapshot,
		() => stores.wallet.snapshot(),
	);
};

export const useSymmTradePanelRows = () => {
	const stores = useSymmTelemetryStores();

	return useSyncExternalStore(
		stores.trades.subscribe,
		stores.trades.snapshot,
		() => stores.trades.snapshot(),
	);
};

export const useSymmAuditRows = () => {
	const stores = useSymmTelemetryStores();

	return useSyncExternalStore(
		stores.audit.subscribe,
		stores.audit.snapshot,
		() => stores.audit.snapshot(),
	);
};

export const useSymmDecisionTrace = () => {
	const stores = useSymmTelemetryStores();

	return useSyncExternalStore(
		stores.decisions.subscribe,
		stores.decisions.snapshot,
		() => undefined,
	);
};
