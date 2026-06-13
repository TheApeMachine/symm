import { useSelector } from "@tanstack/react-store";

import { appStore } from "#/collections/app";
import { AuditDataProvider } from "#/components/panels/data/audit-data-provider";
import { TradeHistoryDataProvider } from "#/components/panels/data/trade-history-data-provider";
import { TradesDataProvider } from "#/components/panels/data/trades-data-provider";
import { WalletDataProvider } from "#/components/panels/data/wallet-data-provider";
import { useStoreSnapshot } from "#/lib/symm/use-store-snapshot";

export const useSymmConnected = () =>
	useSelector(appStore, (state) => state.online);

export const useSymmTick = () =>
	useSelector(appStore, (state) => state.storyTicks);

export const useSymmAuditRows = () => useStoreSnapshot(AuditDataProvider);

export const useSymmTradePanelRows = () => useStoreSnapshot(TradesDataProvider);

export const useSymmTradeHistoryRows = () =>
	useStoreSnapshot(TradeHistoryDataProvider);

export const useSymmWallet = () => useStoreSnapshot(WalletDataProvider);

export const useSymmTelemetryStatus = () => ({ throttled: false });
