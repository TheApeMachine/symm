import { useSyncExternalStore } from "react";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";
import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";
import { WalletDataProvider } from "#/components/symm/wallet-data-provider";
import { ConnectionStore } from "#/lib/symm/connection-store";
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

export const useSymmEnginePulse = () =>
	useSyncExternalStore(
		PredictionsDataProvider.subscribe,
		PredictionsDataProvider.snapshot,
		() => undefined,
	);

export const useSymmWallet = () =>
	useSyncExternalStore(
		WalletDataProvider.subscribe,
		WalletDataProvider.snapshot,
		() => WalletDataProvider.snapshot(),
	);

export const useSymmTradePanelRows = () =>
	useSyncExternalStore(
		TradesDataProvider.subscribe,
		TradesDataProvider.snapshot,
		() => TradesDataProvider.snapshot(),
	);

export const useSymmAuditRows = () =>
	useSyncExternalStore(
		AuditDataProvider.subscribe,
		AuditDataProvider.snapshot,
		() => AuditDataProvider.snapshot(),
	);
