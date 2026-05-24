export const SIGNAL_SOURCES = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"basis",
	"sentiment",
] as const;

export type SignalSource = (typeof SIGNAL_SOURCES)[number];

export type SignalConfidenceSnapshot = Record<SignalSource, number>;

export const SIGNAL_LABELS: Record<SignalSource, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
	depthflow: "Depth",
	leadlag: "LeadLag",
	basis: "Basis",
	sentiment: "Sent",
};

export function isSignalSource(source: string): source is SignalSource {
	return SIGNAL_SOURCES.includes(source as SignalSource);
}

export function confidenceToGaugePercent(confidence: number): number {
	if (confidence <= 0) {
		return 0;
	}

	return Math.min(100, confidence * 100);
}

export function formatSignalConfidence(confidence: number): string {
	if (confidence <= 0) {
		return "0";
	}

	if (confidence >= 100) {
		return confidence.toFixed(0);
	}

	if (confidence >= 1) {
		return confidence.toFixed(2);
	}

	return confidence.toFixed(3);
}

export const emptySignalConfidences = (): SignalConfidenceSnapshot => ({
	hawkes: 0,
	fluid: 0,
	pumpdump: 0,
	causal: 0,
	depthflow: 0,
	leadlag: 0,
	basis: 0,
	sentiment: 0,
});
