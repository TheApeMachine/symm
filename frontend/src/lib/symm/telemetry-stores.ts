import {
	AuditDataProvider,
	createAuditDataProvider,
} from "#/components/symm/audit-data-provider";
import {
	ConfidenceDataProvider,
	createConfidenceDataProvider,
} from "#/components/symm/confidence-data-provider";
import {
	createDecisionsDataProvider,
	DecisionsDataProvider,
} from "#/components/symm/decisions-data-provider";
import {
	createFluidDataProvider,
	FluidDataProvider,
} from "#/components/symm/fluid-data-provider";
import {
	createOhlcDataProvider,
	OhlcDataProvider,
} from "#/components/symm/ohlc-data-provider";
import {
	createPredictionsDataProvider,
	PredictionsDataProvider,
} from "#/components/symm/predictions-data-provider";
import {
	createTradesDataProvider,
	TradesDataProvider,
} from "#/components/symm/trades-data-provider";
import {
	createWalletDataProvider,
	WalletDataProvider,
} from "#/components/symm/wallet-data-provider";

export type SymmTelemetryStores = {
	predictions: ReturnType<typeof createPredictionsDataProvider>;
	audit: ReturnType<typeof createAuditDataProvider>;
	trades: ReturnType<typeof createTradesDataProvider>;
	wallet: ReturnType<typeof createWalletDataProvider>;
	ohlc: ReturnType<typeof createOhlcDataProvider>;
	fluid: ReturnType<typeof createFluidDataProvider>;
	confidence: ReturnType<typeof createConfidenceDataProvider>;
	decisions: ReturnType<typeof createDecisionsDataProvider>;
};

export const createSymmTelemetryStores = (): SymmTelemetryStores => ({
	predictions: createPredictionsDataProvider(),
	audit: createAuditDataProvider(),
	trades: createTradesDataProvider(),
	wallet: createWalletDataProvider(),
	ohlc: createOhlcDataProvider(),
	fluid: createFluidDataProvider(),
	confidence: createConfidenceDataProvider(),
	decisions: createDecisionsDataProvider(),
});

export const defaultSymmTelemetryStores: SymmTelemetryStores = {
	predictions: PredictionsDataProvider,
	audit: AuditDataProvider,
	trades: TradesDataProvider,
	wallet: WalletDataProvider,
	ohlc: OhlcDataProvider,
	fluid: FluidDataProvider,
	confidence: ConfidenceDataProvider,
	decisions: DecisionsDataProvider,
};
