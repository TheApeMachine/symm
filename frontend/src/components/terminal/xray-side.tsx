import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import { measurementsForSource } from "#/collections/measurements";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

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

type FactRefs = {
	regime: HTMLSpanElement | null;
	coherence: HTMLSpanElement | null;
	energy: HTMLSpanElement | null;
	surprise: HTMLSpanElement | null;
	events: HTMLSpanElement | null;
	branching: HTMLSpanElement | null;
};

/*
XrayFactsPanel paints resonance / cognitive / hawkes side facts from stores.
*/
export const XrayFactsPanel = () => {
	const regimeRef = useRef<HTMLSpanElement>(null);
	const coherenceRef = useRef<HTMLSpanElement>(null);
	const energyRef = useRef<HTMLSpanElement>(null);
	const surpriseRef = useRef<HTMLSpanElement>(null);
	const eventsRef = useRef<HTMLSpanElement>(null);
	const branchingRef = useRef<HTMLSpanElement>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "resonance", key: focusSymbol },
			{ store: "manifold", key: focusSymbol },
			{ store: "measurements", key: focusSymbol },
			{ store: "cognitive", key: "" },
		],
		(buffers) => {
			const refs: FactRefs = {
				regime: regimeRef.current,
				coherence: coherenceRef.current,
				energy: energyRef.current,
				surprise: surpriseRef.current,
				events: eventsRef.current,
				branching: branchingRef.current,
			};
			const resonance = (
				(buffers[`resonance:${focusSymbol}`] ?? []).at(-1) ?? null
			) as ResonanceFrame | null;
			const manifold = (
				(buffers[`manifold:${focusSymbol}`] ?? []).at(-1) ?? null
			) as ManifoldFrame | null;
			const hawkesFrames = measurementsForSource(
				(buffers[`measurements:${focusSymbol}`] ?? []) as Measurement[],
				"hawkes",
			);
			const reading = manifoldReading(manifold);
			const hawkes = hawkesMetricsFromBuffer(hawkesFrames);
			const cascade = cascadeLabel(hawkes.branching);
			const cognitive = cognitiveForSymbol(
				(buffers["cognitive:"] ?? []) as CognitiveReading[],
				focusSymbol,
			);
			const coherenceMag2 = finiteMetric(reading?.coherenceMag2);
			const coherence =
				coherenceMag2 === null
					? "—"
					: coherenceMag2 >= 0.4
						? "laminar"
						: "turbulent";
			const surprise = finiteMetric(resonance?.surprise);
			const events = hawkesEventCount(hawkesFrames);

			if (refs.regime !== null) {
				refs.regime.textContent =
					cognitive?.regimePrefix || stringMetric(resonance?.category) || "—";
			}

			if (refs.coherence !== null) {
				refs.coherence.textContent = coherence;
				refs.coherence.style.color =
					coherence === "laminar"
						? "var(--info)"
						: coherence === "turbulent"
							? "var(--down)"
							: "var(--f4)";
			}

			if (refs.energy !== null) {
				refs.energy.textContent = formatMetric(finiteMetric(resonance?.energy));
			}

			if (refs.surprise !== null) {
				refs.surprise.textContent =
					surprise === null ? "—" : `${surprise.toFixed(2)}× thr`;
			}

			if (refs.events !== null) {
				refs.events.textContent = events === 0 ? "—" : String(events);
			}

			if (refs.branching !== null) {
				refs.branching.textContent = formatMetric(hawkes.branching);
				refs.branching.style.color = cascade.color;
			}
		},
		[online, focusSymbol],
	);

	return (
		<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">regime class</span>
				<span ref={regimeRef} className="text-right text-(--acc)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">coherence</span>
				<span ref={coherenceRef} className="text-right" />
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
};

type ManifoldRefs = {
	divergence: HTMLSpanElement | null;
	coherence: HTMLSpanElement | null;
	guidance: HTMLSpanElement | null;
	viscosity: HTMLSpanElement | null;
	momentum: HTMLSpanElement | null;
	fill: HTMLDivElement | null;
};

/*
XrayManifoldPanel paints nested manifold.reading scalars from the store.
*/
export const XrayManifoldPanel = () => {
	const divergenceRef = useRef<HTMLSpanElement>(null);
	const coherenceRef = useRef<HTMLSpanElement>(null);
	const guidanceRef = useRef<HTMLSpanElement>(null);
	const viscosityRef = useRef<HTMLSpanElement>(null);
	const momentumRef = useRef<HTMLSpanElement>(null);
	const fillRef = useRef<HTMLDivElement>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "manifold", key: focusSymbol },
			{ store: "measurements", key: focusSymbol },
		],
		(buffers) => {
			const refs: ManifoldRefs = {
				divergence: divergenceRef.current,
				coherence: coherenceRef.current,
				guidance: guidanceRef.current,
				viscosity: viscosityRef.current,
				momentum: momentumRef.current,
				fill: fillRef.current,
			};
			const frame = (
				(buffers[`manifold:${focusSymbol}`] ?? []).at(-1) ?? null
			) as ManifoldFrame | null;
			const reading = manifoldReading(frame);
			const hawkes = hawkesMetricsFromBuffer(
				measurementsForSource(
					(buffers[`measurements:${focusSymbol}`] ?? []) as Measurement[],
					"hawkes",
				),
			);
			const momentumShare =
				finiteMetric(reading?.coherenceMag2) ??
				hawkes.radius ??
				hawkes.branching ??
				0;

			if (refs.divergence !== null) {
				refs.divergence.textContent = signedMetric(
					finiteMetric(reading?.divergence),
				);
			}

			if (refs.coherence !== null) {
				refs.coherence.textContent = formatMetric(
					finiteMetric(reading?.coherenceMag2),
				);
			}

			if (refs.guidance !== null) {
				const latticeGuidance = meanGuidanceSpeed(
					frameMatrix(frame, "guidanceVelX"),
					frameMatrix(frame, "guidanceVelZ"),
				);
				refs.guidance.textContent = formatMetric(
					finiteMetric(reading?.guidanceSpeed) ?? latticeGuidance,
				);
			}

			if (refs.viscosity !== null) {
				refs.viscosity.textContent = formatMetric(
					finiteMetric(reading?.viscosityProxy),
				);
			}

			if (refs.momentum !== null) {
				refs.momentum.textContent = `${momentumShare.toFixed(2)} / 0.40`;
				refs.momentum.style.color =
					momentumShare >= 0.4 ? "var(--up)" : "var(--f3)";
			}

			if (refs.fill !== null) {
				refs.fill.style.width = `${Math.min(100, momentumShare * 100)}%`;
				refs.fill.style.background =
					momentumShare >= 0.4 ? "var(--success)" : "var(--info)";
			}
		},
		[online, focusSymbol],
	);

	return (
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
					<span ref={coherenceRef} className="text-right text-(--f1)" />
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
};
