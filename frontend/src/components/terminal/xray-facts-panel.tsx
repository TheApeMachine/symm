import { createRef } from "react";
import type {
	CognitiveReading,
	ManifoldFrame,
	Measurement,
	ResonanceFrame,
} from "#/collections/types";
import {
	cascadeLabel,
	finiteMetric,
	formatMetric,
	hawkesEventCount,
	hawkesMetricsFromBuffer,
	manifoldReading,
	stringMetric,
} from "#/components/terminal/xray-view";
import { frameRows } from "#/providers/frame-history";

const regimeRef = createRef<HTMLSpanElement>();
const coherenceFactRef = createRef<HTMLSpanElement>();
const energyRef = createRef<HTMLSpanElement>();
const surpriseRef = createRef<HTMLSpanElement>();
const eventsRef = createRef<HTMLSpanElement>();
const branchingRef = createRef<HTMLSpanElement>();

const cognitiveScopes = (readings: CognitiveReading[]): string[] =>
	[
		...new Set(
			readings
				.map((reading) => reading.symbol || reading.scope || "")
				.filter((scope) => scope !== ""),
		),
	].sort();

const cognitiveForSymbol = (
	readings: CognitiveReading[],
	symbol: string,
): CognitiveReading | null => {
	if (symbol !== "") {
		return readings.find((reading) => reading.symbol === symbol) ?? null;
	}

	const [scope] = cognitiveScopes(readings);

	return scope === undefined
		? null
		: (readings.find((reading) => reading.symbol === scope) ?? null);
};

const focusedFrame = <T extends { symbol: string }>(
	value: unknown,
	focusSymbol: string,
): T | null => {
	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as T[];

	return (
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ??
		frames.at(-1) ??
		null
	);
};

/*
paintXrayFactsResonance paints regime / energy / surprise from the current
DRAW resonance batch into the facts panel.
*/
export const paintXrayFactsResonance = (
	value: unknown,
	focusSymbol: string,
) => {
	const resonance = focusedFrame<ResonanceFrame>(value, focusSymbol);
	const surprise = finiteMetric(resonance?.surprise);

	if (regimeRef.current !== null) {
		regimeRef.current.textContent = stringMetric(resonance?.category) || "—";
	}

	if (energyRef.current !== null) {
		energyRef.current.textContent = formatMetric(
			finiteMetric(resonance?.energy),
		);
	}

	if (surpriseRef.current !== null) {
		surpriseRef.current.textContent =
			surprise === null ? "—" : `${surprise.toFixed(2)}× thr`;
	}
};

/*
paintXrayFactsManifold paints coherence from the current DRAW manifold batch
into the facts panel.
*/
export const paintXrayFactsManifold = (value: unknown, focusSymbol: string) => {
	const manifold = focusedFrame<ManifoldFrame>(value, focusSymbol);
	const reading = manifoldReading(manifold);
	const coherenceMag2 = finiteMetric(reading?.coherenceMag2);
	const coherence =
		coherenceMag2 === null
			? "—"
			: coherenceMag2 >= 0.4
				? "laminar"
				: "turbulent";

	if (coherenceFactRef.current !== null) {
		coherenceFactRef.current.textContent = coherence;
		coherenceFactRef.current.style.color =
			coherence === "laminar"
				? "var(--info)"
				: coherence === "turbulent"
					? "var(--down)"
					: "var(--f4)";
	}
};

/*
paintXrayFactsMeasurements paints cumulative flow-event and current branching
facts from retained Hawkes measurement history.
*/
export const paintXrayFactsMeasurements = (
	value: unknown,
	focusSymbol: string,
) => {
	if (eventsRef.current === null) {
		return;
	}

	const measurements = frameRows<Measurement>(value);
	const hawkesFrames = measurements.filter(
		(measurement) =>
			measurement.source === "hawkes" &&
			(focusSymbol === "" || measurement.symbol === focusSymbol),
	);
	const hawkes = hawkesMetricsFromBuffer(hawkesFrames);
	const cascade = cascadeLabel(hawkes.branching);
	const events = hawkesEventCount(hawkesFrames);

	if (eventsRef.current !== null) {
		eventsRef.current.textContent = events === 0 ? "—" : String(events);
	}

	if (branchingRef.current !== null) {
		branchingRef.current.textContent = formatMetric(hawkes.branching);
		branchingRef.current.style.color = cascade.color;
	}
};

/*
paintXrayFactsCognition paints regime class from the current DRAW cognition
batch into the facts panel.
*/
export const paintXrayFactsCognition = (
	value: unknown,
	focusSymbol: string,
) => {
	const readings = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as CognitiveReading[];
	const cognitive = cognitiveForSymbol(readings, focusSymbol);

	if (regimeRef.current !== null && cognitive?.regimePrefix) {
		regimeRef.current.textContent = cognitive.regimePrefix;
	}
};

/*
XrayFactsPanel is the static facts shell. DRAW paints via the paintXrayFacts*
exports.
*/
export const XrayFactsPanel = () => (
	<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">regime class</span>
			<span ref={regimeRef} className="text-right text-(--acc)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">coherence</span>
			<span ref={coherenceFactRef} className="text-right" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">free energy</span>
			<span ref={energyRef} className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">surprise</span>
			<span ref={surpriseRef} className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">flow events</span>
			<span ref={eventsRef} className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">branching η</span>
			<span ref={branchingRef} className="text-right" />
		</div>
	</div>
);
