import { createRef } from "react";
import type {
	CognitiveReading,
	ManifoldFrame,
	Measurement,
	ResonanceFrame,
} from "#/collections/types";
import {
	isFluidFieldMatrix,
	meanGuidanceSpeed,
} from "#/components/terminal/fluid-field";
import {
	cascadeLabel,
	finiteMetric,
	formatMetric,
	hawkesEventCount,
	hawkesMetricsFromBuffer,
	manifoldReading,
	signedMetric,
	stringMetric,
} from "#/components/terminal/xray-view";

const regimeRef = createRef<HTMLSpanElement>();
const coherenceFactRef = createRef<HTMLSpanElement>();
const energyRef = createRef<HTMLSpanElement>();
const surpriseRef = createRef<HTMLSpanElement>();
const eventsRef = createRef<HTMLSpanElement>();
const branchingRef = createRef<HTMLSpanElement>();

const divergenceRef = createRef<HTMLSpanElement>();
const coherenceManifoldRef = createRef<HTMLSpanElement>();
const guidanceRef = createRef<HTMLSpanElement>();
const viscosityRef = createRef<HTMLSpanElement>();
const momentumRef = createRef<HTMLSpanElement>();
const fillRef = createRef<HTMLDivElement>();

const numberMatrix = (value: unknown): number[][] =>
	Array.isArray(value)
		? value
				.map((row) =>
					Array.isArray(row)
						? row.filter((cell): cell is number => Number.isFinite(cell))
						: [],
				)
				.filter((row) => row.length > 0)
		: [];

const frameMatrix = (frame: unknown, key: string): number[][] | undefined => {
	if (frame === null || frame === undefined || typeof frame !== "object") {
		return undefined;
	}

	const value = (frame as Record<string, unknown>)[key];
	const matrix = numberMatrix(value);

	return isFluidFieldMatrix(matrix) ? matrix : undefined;
};

/*
cognitiveScopes lists symbols that currently own a cognitive reading.
*/
export const cognitiveScopes = (readings: CognitiveReading[]): string[] =>
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
	const measurements = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
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

const writeMomentum = (momentumShare: number) => {
	if (momentumRef.current !== null) {
		momentumRef.current.textContent = `${momentumShare.toFixed(2)} / 0.40`;
		momentumRef.current.style.color =
			momentumShare >= 0.4 ? "var(--up)" : "var(--f3)";
	}

	if (fillRef.current !== null) {
		fillRef.current.style.width = `${Math.min(100, momentumShare * 100)}%`;
		fillRef.current.style.background =
			momentumShare >= 0.4 ? "var(--success)" : "var(--info)";
	}
};

/*
paintXrayManifold paints nested manifold.reading scalars from the current DRAW
manifold batch into the manifold side panel.
*/
export const paintXrayManifold = (value: unknown, focusSymbol: string) => {
	const frame = focusedFrame<ManifoldFrame>(value, focusSymbol);
	const reading = manifoldReading(frame);
	const momentumShare = finiteMetric(reading?.coherenceMag2) ?? 0;

	if (divergenceRef.current !== null) {
		divergenceRef.current.textContent = signedMetric(
			finiteMetric(reading?.divergence),
		);
	}

	if (coherenceManifoldRef.current !== null) {
		coherenceManifoldRef.current.textContent = formatMetric(
			finiteMetric(reading?.coherenceMag2),
		);
	}

	if (guidanceRef.current !== null) {
		const latticeGuidance = meanGuidanceSpeed(
			frameMatrix(frame, "guidanceVelX"),
			frameMatrix(frame, "guidanceVelZ"),
		);
		guidanceRef.current.textContent = formatMetric(
			finiteMetric(reading?.guidanceSpeed) ?? latticeGuidance,
		);
	}

	if (viscosityRef.current !== null) {
		viscosityRef.current.textContent = formatMetric(
			finiteMetric(reading?.viscosityProxy),
		);
	}

	writeMomentum(momentumShare);
};

/*
paintXrayManifoldMeasurements updates momentum eigenmode from retained Hawkes
history when manifold coherence is unavailable.
*/
export const paintXrayManifoldMeasurements = (
	value: unknown,
	focusSymbol: string,
) => {
	const measurements = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
	const hawkes = hawkesMetricsFromBuffer(
		measurements.filter(
			(measurement) =>
				measurement.source === "hawkes" &&
				(focusSymbol === "" || measurement.symbol === focusSymbol),
		),
	);
	const share = hawkes.radius ?? hawkes.branching;

	if (share === null) {
		return;
	}

	const coherenceText = coherenceManifoldRef.current?.textContent ?? "";

	if (coherenceText !== "" && coherenceText !== "—") {
		return;
	}

	writeMomentum(share);
};

/*
XrayManifoldPanel is the static manifold reading shell. DRAW paints via
paintXrayManifold and paintXrayManifoldMeasurements.
*/
export const XrayManifoldPanel = () => (
	<div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
		<div>
			<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				Manifold reading
			</div>
			<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
				|ψ|² · guidance current · particles
			</div>
		</div>
		<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">∇·u</span>
				<span ref={divergenceRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">|ψ|²</span>
				<span ref={coherenceManifoldRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">guide v</span>
				<span ref={guidanceRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">viscosity</span>
				<span ref={viscosityRef} className="text-right text-(--f1)" />
			</div>
		</div>
		<div className="mt-0.5">
			<div className="mb-1 flex justify-between text-[10px]">
				<span className="text-(--f3)">momentum eigenmode</span>
				<span ref={momentumRef} className="font-mono" />
			</div>
			<div className="relative">
				<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
					<div ref={fillRef} className="h-full" style={{ width: "0%" }} />
				</div>
				<div className="relative h-0">
					<div className="absolute top-[-9px] left-[40%] h-3 w-0.5 bg-(--acc)" />
				</div>
			</div>
			<div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
				drive playbook gate · mode share ≥ 0.40
			</div>
		</div>
	</div>
);
