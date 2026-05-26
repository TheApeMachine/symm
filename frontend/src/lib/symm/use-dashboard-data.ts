import { useSyncExternalStore } from "react";

import { ConnectionStore } from "#/lib/symm/connection-store";
import { EnginePulseDataProvider } from "#/components/symm/engine-pulse-data-provider";
import { WalletDataProvider } from "#/components/symm/wallet-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";

export const useSymmConnected = () =>
	useSyncExternalStore(
		ConnectionStore.subscribe,
		() => ConnectionStore.snapshot(),
		() => false,
	);

export const useSymmEnginePulse = () =>
	useSyncExternalStore(
		EnginePulseDataProvider.subscribe,
		EnginePulseDataProvider.snapshot,
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
