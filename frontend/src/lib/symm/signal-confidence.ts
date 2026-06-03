// Hub confidence frames carry category-band clarity on 0..1. SNR (sigma surprise)
// rides alongside as snr when present. The gauge maps clarity onto 0..100%.
export const confidenceToGaugePercent = (confidence: number): number => {
	if (!Number.isFinite(confidence) || confidence <= 0) {
		return 0;
	}

	if (confidence > 1) {
		throw new Error(`signal confidence out of unit interval: ${confidence}`);
	}

	return confidence * 100;
};

export const formatSignalConfidence = (confidence: number): string => {
	if (confidence <= 0) {
		return "0";
	}

	return confidenceToGaugePercent(confidence).toFixed(1);
};
