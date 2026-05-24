import type { EnginePulseEvent } from "#/lib/symm/events";

export const SIGNAL_SOURCES = [
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
] as const;

export type SignalSource = (typeof SIGNAL_SOURCES)[number];

export type SignalConfidenceSnapshot = Record<SignalSource, number>;

export const SIGNAL_LABELS: Record<SignalSource, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
};

const emptySnapshot = (): SignalConfidenceSnapshot => ({
	hawkes: 0,
	fluid: 0,
	pumpdump: 0,
	causal: 0,
});

function isSignalSource(source: string): source is SignalSource {
	return SIGNAL_SOURCES.includes(source as SignalSource);
}

/** Peak score per source from one engine pulse payload. */
export function peakSignalConfidencesFromPulse(
	pulse: EnginePulseEvent | undefined,
): SignalConfidenceSnapshot {
	const peaks = emptySnapshot();

	for (const row of pulse?.signals ?? []) {
		if (!isSignalSource(row.source) || row.score <= peaks[row.source]) {
			continue;
		}

		peaks[row.source] = row.score;
	}

	return peaks;
}

/** Hold the latest reported score per source from live track telemetry. */
export function mergeSignalConfidences(
	current: SignalConfidenceSnapshot,
	pulse: EnginePulseEvent,
): SignalConfidenceSnapshot {
	const next = { ...current };

	if (pulse.source_scores) {
		for (const source of SIGNAL_SOURCES) {
			const score = pulse.source_scores[source];

			if (score !== undefined && Number.isFinite(score) && score > 0) {
				next[source] = score;
			}
		}
	}

	const pulsePeaks = peakSignalConfidencesFromPulse(pulse);

	for (const source of SIGNAL_SOURCES) {
		if (pulsePeaks[source] <= 0) {
			continue;
		}

		next[source] = Math.max(next[source], pulsePeaks[source]);
	}

	return next;
}

/** Map unit-scale confidence (0–1) to a 0–100 gauge needle. */
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
