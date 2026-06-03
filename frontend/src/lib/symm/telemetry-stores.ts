import {
	ConfidenceDataProvider,
	createConfidenceDataProvider,
} from "#/components/charts/confidence/confidence-data-provider";
import {
	createFluidDataProvider,
	FluidDataProvider,
} from "#/components/charts/fluid/fluid-data-provider";
import {
	createPredictionsDataProvider,
	PredictionsDataProvider,
} from "#/components/charts/prediction/predictions-data-provider";
import {
	AuditDataProvider,
	createAuditDataProvider,
} from "#/components/panels/data/audit-data-provider";
import {
	createDecisionsDataProvider,
	DecisionsDataProvider,
} from "#/components/panels/data/decisions-data-provider";
import {
	createTradesDataProvider,
	TradesDataProvider,
} from "#/components/panels/data/trades-data-provider";
import {
	createWalletDataProvider,
	WalletDataProvider,
} from "#/components/panels/data/wallet-data-provider";

export type SymmTelemetryStores = {
	predictions: ReturnType<typeof createPredictionsDataProvider>;
	audit: ReturnType<typeof createAuditDataProvider>;
	trades: ReturnType<typeof createTradesDataProvider>;
	wallet: ReturnType<typeof createWalletDataProvider>;
	fluid: ReturnType<typeof createFluidDataProvider>;
	confidence: ReturnType<typeof createConfidenceDataProvider>;
	decisions: ReturnType<typeof createDecisionsDataProvider>;
};

export const createSymmTelemetryStores = (): SymmTelemetryStores => ({
	predictions: createPredictionsDataProvider(),
	audit: createAuditDataProvider(),
	trades: createTradesDataProvider(),
	wallet: createWalletDataProvider(),
	fluid: createFluidDataProvider(),
	confidence: createConfidenceDataProvider(),
	decisions: createDecisionsDataProvider(),
});

export const defaultSymmTelemetryStores: SymmTelemetryStores = {
	predictions: PredictionsDataProvider,
	audit: AuditDataProvider,
	trades: TradesDataProvider,
	wallet: WalletDataProvider,
	fluid: FluidDataProvider,
	confidence: ConfidenceDataProvider,
	decisions: DecisionsDataProvider,
};
