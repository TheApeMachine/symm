export const SIGNAL_SOURCES = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"liquidity",
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
	liquidity: "Basis",
	sentiment: "Sent",
};

export const isSignalSource = (source: string): source is SignalSource =>
	SIGNAL_SOURCES.includes(source as SignalSource);

// Confidence is now a signal-to-noise ratio in noise-sigma units. The gauge maps
// 0..GAUGE_FULL_SIGMA sigma onto 0..100% and caps there, so a reading sits at the
// noise floor (1 sigma) around a quarter of the dial and pins at strong spikes
// instead of running off to thousands.
const GAUGE_FULL_SIGMA = 4;

export const confidenceToGaugePercent = (confidence: number): number => {
	if (confidence <= 0) {
		return 0;
	}

	return Math.min(100, (confidence / GAUGE_FULL_SIGMA) * 100);
};

export const formatSignalConfidence = (confidence: number): string => {
	if (confidence <= 0) {
		return "0";
	}

	return confidenceToGaugePercent(confidence).toFixed(1);
};

export const emptySignalConfidences = (): SignalConfidenceSnapshot => ({
	hawkes: 0,
	fluid: 0,
	pumpdump: 0,
	causal: 0,
	depthflow: 0,
	leadlag: 0,
	liquidity: 0,
	sentiment: 0,
});
