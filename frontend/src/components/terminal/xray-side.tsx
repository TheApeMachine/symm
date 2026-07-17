import { useRef } from "react";
import { appStore } from "#/collections/app";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
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

const isConcreteSymbol = (symbol: string): boolean => symbol !== "";

const cognitiveForSymbol = (
	readings: Record<string, CognitiveReading>,
	symbol: string,
): CognitiveReading | null => {
	if (isConcreteSymbol(symbol)) {
		return readings[symbol] ?? null;
	}

	const [scope] = cognitiveScopes(readings);

	return scope === undefined ? null : readings[scope];
};

type FactRefs = {
	regime: HTMLSpanElement | null;
	coherence: HTMLSpanElement | null;
	energy: HTMLSpanElement | null;
	surprise: HTMLSpanElement | null;
	events: HTMLSpanElement | null;
	branching: HTMLSpanElement | null;
};

const paintFacts = (refs: FactRefs): void => {
	const symbol = appStore.state.focusSymbol;
	const resonance =
		resonanceStore.state.resonance[symbol]?.values().at(-1) ?? null;
	const manifold =
		manifoldStore.state.manifold[symbol]?.values().at(-1) ?? null;
	const reading = manifoldReading(manifold);
	const hawkes = hawkesMetricsFromBuffer(
		measurementsStore.state.measurements[symbol]?.hawkes,
	);
	const cascade = cascadeLabel(hawkes.branching);
	const cognitive = cognitiveForSymbol(cognitiveStore.state.readings, symbol);
	const coherenceMag2 = finiteMetric(reading?.coherenceMag2);
	const coherence =
		coherenceMag2 === null
			? "—"
			: coherenceMag2 >= 0.4
				? "laminar"
				: "turbulent";
	const surprise = finiteMetric(resonance?.surprise);
	const events = hawkesEventCount(
		measurementsStore.state.measurements[symbol]?.hawkes,
	);

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

	useDirectStorePaint(
		() =>
			paintFacts({
				regime: regimeRef.current,
				coherence: coherenceRef.current,
				energy: energyRef.current,
				surprise: surpriseRef.current,
				events: eventsRef.current,
				branching: branchingRef.current,
			}),
		[resonanceStore, manifoldStore, measurementsStore, cognitiveStore, appStore],
		[],
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

const paintManifold = (refs: ManifoldRefs): void => {
	const symbol = appStore.state.focusSymbol;
	const frame = manifoldStore.state.manifold[symbol]?.values().at(-1) ?? null;
	const reading = manifoldReading(frame);
	const hawkes = hawkesMetricsFromBuffer(
		measurementsStore.state.measurements[symbol]?.hawkes,
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
		refs.guidance.textContent = formatMetric(
			finiteMetric(reading?.guidanceSpeed),
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

	useDirectStorePaint(
		() =>
			paintManifold({
				divergence: divergenceRef.current,
				coherence: coherenceRef.current,
				guidance: guidanceRef.current,
				viscosity: viscosityRef.current,
				momentum: momentumRef.current,
				fill: fillRef.current,
			}),
		[manifoldStore, measurementsStore, appStore],
		[],
	);

	return (
		<div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
			<div>
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Manifold reading
				</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
					navier–stokes · ρ projection · oscillator carriers
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
