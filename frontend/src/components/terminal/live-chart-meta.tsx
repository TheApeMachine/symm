import { createRef } from "react";
import { terminalStore } from "#/collections/terminal";
import type { ManifoldFrame, ResonanceFrame } from "#/collections/types";
import {
	finiteNumber,
	fluidGridDimensions,
	frameAuxMatrix,
	terminalFluidDisplayLatticeFromFrame,
	terminalFluidParticlesFromFrame,
	withSharedManifoldField,
} from "#/components/terminal/charts-frame";
import {
	isFluidFieldMatrix,
	terminalFluidFieldStats,
} from "#/components/terminal/fluid-field";
import { aggregateFluidParticles } from "#/components/terminal/fluid-particles";
import { latestManifoldParticles } from "#/providers/manifold-parts";

const manifoldWaitingRef = createRef<HTMLDivElement>();
const manifoldGridRef = createRef<HTMLDivElement>();
const manifoldPopulationRef = createRef<HTMLDivElement>();
const manifoldProjectionRef = createRef<HTMLDivElement>();
const manifoldCoherenceRef = createRef<HTMLDivElement>();
const manifoldGasRef = createRef<HTMLDivElement>();
let manifoldMetaFocus = "";
let manifoldMetaLayer = terminalStore.state.fieldLayer;

const resonanceFooterRef = createRef<HTMLSpanElement>();
const resonanceTitleRef = createRef<HTMLSpanElement>();

const formatFieldMaximum = (value: number): string =>
	new Intl.NumberFormat("en", {
		maximumSignificantDigits: 3,
		notation: "scientific",
	}).format(value);

/*
paintManifoldMeta updates metadata only from a complete focused projection.
Summary deltas cannot replace valid grid statistics with a misleading 0×0
field, while a focus change returns the shell to an explicit waiting state.
*/
export const paintManifoldMeta = (value: unknown, focusSymbol: string) => {
	const focusChanged = manifoldMetaFocus !== focusSymbol;
	manifoldMetaFocus = focusSymbol;
	const layer = terminalStore.state.fieldLayer;
	const layerChanged = manifoldMetaLayer !== layer;
	manifoldMetaLayer = layer;
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ManifoldFrame[];
	const focused = rows
		.filter((frame) => focusSymbol === "" || frame.symbol === focusSymbol)
		.at(-1);
	const manifold = withSharedManifoldField(focused ?? null, rows);
	const display = terminalFluidDisplayLatticeFromFrame(manifold ?? null, layer);
	const field = isFluidFieldMatrix(display) ? display : [];

	if (field.length === 0 && !focusChanged && !layerChanged) {
		return;
	}

	const waiting = field.length === 0;
	const { columns, rows: gridRows } = fluidGridDimensions(
		manifold ?? null,
		field,
	);
	const particles = terminalFluidParticlesFromFrame({
		...(manifold ?? {}),
		particles: latestManifoldParticles()?.particles ?? manifold?.particles,
	});
	const particleCells = aggregateFluidParticles(particles, columns, gridRows);
	const coherence = terminalFluidFieldStats(
		frameAuxMatrix(manifold ?? null, "psiMag2"),
	);
	const gas = terminalFluidFieldStats(frameAuxMatrix(manifold ?? null, "rho"));
	const focusedCount = finiteNumber(manifold?.oscillatorCount);
	const sharedCount = finiteNumber(manifold?.sharedOscillatorCount);
	const focusedLabel = String(focusedCount ?? particles.length);
	const sharedLabel =
		sharedCount === null ? "unavailable" : String(sharedCount);
	const hidden = waiting ? "none" : "";
	const lines = [
		[manifoldWaitingRef, waiting ? "" : "none", "waiting"],
		[manifoldGridRef, hidden, `grid ${String(columns)}×${String(gridRows)}`],
		[
			manifoldPopulationRef,
			hidden,
			`particles ${focusedLabel} focused · ${sharedLabel} shared`,
		],
		[
			manifoldProjectionRef,
			hidden,
			`focused projection ${String(particleCells.length)} occupied X–Z cells`,
		],
		[
			manifoldCoherenceRef,
			hidden,
			`|ψ|² ${String(coherence.occupied)} active · max ${formatFieldMaximum(coherence.maximum)}`,
		],
		[
			manifoldGasRef,
			hidden,
			`gas ρ ${String(gas.occupied)} active · max ${formatFieldMaximum(gas.maximum)}`,
		],
	] as const;

	for (const [ref, display, text] of lines) {
		if (ref.current === null) {
			continue;
		}

		ref.current.style.display = display;
		ref.current.textContent = text;
	}
};

/*
paintResonanceFooter paints predictive-coding footer metadata from the current
DRAW resonance batch.
*/
export const paintResonanceFooter = (value: unknown, focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ResonanceFrame[];
	const resonance = rows
		.filter((frame) => focusSymbol === "" || frame.symbol === focusSymbol)
		.at(-1);

	if (resonanceFooterRef.current !== null) {
		resonanceFooterRef.current.textContent =
			resonance === undefined
				? "waiting"
				: `symbol ${String(resonance.symbol)}`;
	}
};

/*
paintResonanceTitle paints predictive-coding title samples from the current
DRAW resonance batch.
*/
export const paintResonanceTitle = (value: unknown, focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ResonanceFrame[];
	const resonance = rows
		.filter((frame) => focusSymbol === "" || frame.symbol === focusSymbol)
		.at(-1);

	if (resonanceTitleRef.current !== null) {
		resonanceTitleRef.current.textContent =
			resonance === undefined
				? "waiting"
				: `${String(resonance.samples)} samples`;
	}
};

/*
LiveManifoldMeta is the static pilot-wave metadata shell. DRAW paints via
paintManifoldMeta.
*/
export const LiveManifoldMeta = (_props: { focusSymbol: string }) => (
	<div>
		<div ref={manifoldWaitingRef}>waiting</div>
		<div ref={manifoldGridRef} />
		<div ref={manifoldPopulationRef} />
		<div ref={manifoldProjectionRef} />
		<div ref={manifoldCoherenceRef} />
		<div ref={manifoldGasRef} />
	</div>
);

/*
LiveResonanceFooter is the static predictive-coding footer shell. DRAW paints
via paintResonanceFooter.
*/
export const LiveResonanceFooter = (_props: { focusSymbol: string }) => (
	<span ref={resonanceFooterRef} />
);

/*
LiveResonanceTitle is the static predictive-coding title shell. DRAW paints via
paintResonanceTitle.
*/
export const LiveResonanceTitle = (_props: { focusSymbol: string }) => (
	<span ref={resonanceTitleRef} />
);
