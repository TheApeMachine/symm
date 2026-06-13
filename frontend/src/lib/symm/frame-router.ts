import { applyBalanceFrame, applyPositionFrame } from "#/collections/positions";
import { AuditDataProvider } from "#/components/panels/data/audit-data-provider";
import { DecisionsDataProvider } from "#/components/panels/data/decisions-data-provider";
import { TradeHistoryDataProvider } from "#/components/panels/data/trade-history-data-provider";
import { TradesDataProvider } from "#/components/panels/data/trades-data-provider";
import { WalletDataProvider } from "#/components/panels/data/wallet-data-provider";
import { walletPayloadFromFrame } from "#/lib/symm/events";

const finiteNumber = (value: unknown): number | null => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return null;
	}

	return value;
};

/*
routeWireFrame fans out dashboard websocket frames to panel stores.
*/
export const routeWireFrame = (raw: Record<string, unknown>) => {
	const frameType = typeof raw.type === "string" ? raw.type : "";

	switch (frameType) {
		case "balances":
		case "wallet": {
			applyBalanceFrame(raw);
			WalletDataProvider.ingest(walletPayloadFromFrame(raw));
			TradesDataProvider.ingest(walletPayloadFromFrame(raw));
			TradeHistoryDataProvider.ingestBalance(raw);
			break;
		}
		case "positions":
			applyPositionFrame(raw);
			TradesDataProvider.ingest(raw);
			TradeHistoryDataProvider.ingestPositions(raw);
			break;
		case "audit":
			AuditDataProvider.ingest(raw);
			TradeHistoryDataProvider.ingestAudit(raw);
			break;
		case "decision_walk":
		case "decision_trace":
			DecisionsDataProvider.ingest(raw);
			break;
		case "mark": {
			const symbol = typeof raw.symbol === "string" ? raw.symbol : "";
			const markPrice =
				finiteNumber(raw.price) ??
				finiteNumber(raw.mark) ??
				finiteNumber(raw.last);

			if (symbol !== "" && markPrice !== null && markPrice > 0) {
				TradesDataProvider.setMark(symbol, markPrice);
			}

			break;
		}
		default:
			break;
	}
};
