import { createRef } from "react";
import type { ManifoldFrame, ResonanceFrame } from "#/collections/types";
import {
	finiteNumber,
	fluidGridDimensions,
} from "#/components/terminal/charts-frame";
import { latestDisplay } from "#/providers/manifold-binary";

const manifoldWaitingRef = createRef<HTMLDivElement>();
const manifoldGridRef = createRef<HTMLDivElement>();
const manifoldPopulationRef = createRef<HTMLDivElement>();
const manifoldProjectionRef = createRef<HTMLDivElement>();
const manifoldCoherenceRef = createRef<HTMLDivElement>();
const manifoldGasRef = createRef<HTMLDivElement>();
let manifoldMetaFocus = "";
let lastManifoldMetaBatch: unknown = null;

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
	lastManifoldMetaBatch = value;
	paintManifoldMetaCompose(value, focusSymbol);
};

/*
repaintManifoldMeta refreshes grid stats after a binary lattice arrives without
a new JSON manifold meta frame.
*/
export const repaintManifoldMeta = (focusSymbol: string) => {
	if (lastManifoldMetaBatch === null) {
		return;
	}

	paintManifoldMetaCompose(lastManifoldMetaBatch, focusSymbol);
};

const paintManifoldMetaCompose = (value: unknown, focusSymbol: string) => {
	const focusChanged = manifoldMetaFocus !== focusSymbol;
	manifoldMetaFocus = focusSymbol;
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ManifoldFrame[];
	const focused = rows
		.filter((frame) => focusSymbol === "" || frame.symbol === focusSymbol)
		.at(-1);
	const manifold = focused ?? null;
	const baked = latestDisplay();
	const hasPicture = baked !== null;

	if (!hasPicture && !focusChanged) {
		return;
	}

	const waiting = !hasPicture;
	const grid = fluidGridDimensions(manifold ?? null);
	const columns = baked?.width ?? grid.columns;
	const gridRows = baked?.height ?? grid.rows;
	const focusedCount = finiteNumber(manifold?.oscillatorCount);
	const sharedCount = finiteNumber(manifold?.sharedOscillatorCount);
	const rhoOccupied = finiteNumber(manifold?.rhoOccupied);
	const psiOccupied = finiteNumber(manifold?.psiOccupied);
	const rhoMax = finiteNumber(manifold?.rhoMax);
	const psiMax = finiteNumber(manifold?.psiMax);
	const focusedLabel =
		focusedCount === null ? "unavailable" : String(focusedCount);
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
			`focused projection ${String(rhoOccupied ?? 0)} occupied X–Z cells`,
		],
		[
			manifoldCoherenceRef,
			hidden,
			`|ψ|² ${String(psiOccupied ?? 0)} active · max ${formatFieldMaximum(psiMax ?? 0)}`,
		],
		[
			manifoldGasRef,
			hidden,
			`gas ρ ${String(rhoOccupied ?? 0)} active · max ${formatFieldMaximum(rhoMax ?? 0)}`,
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
