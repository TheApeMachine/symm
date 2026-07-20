import { createRef } from "react";
import type { ManifoldFrame, ResonanceFrame } from "#/collections/types";
import { terminalStore } from "#/collections/terminal";
import {
	fluidGridDimensions,
	terminalFluidDisplayLatticeFromFrame,
} from "#/components/terminal/charts";
import {
	isFluidFieldMatrix,
	terminalFluidFieldStats,
} from "#/components/terminal/fluid-field";

const manifoldWaitingRef = createRef<HTMLDivElement>();
const manifoldGridRef = createRef<HTMLDivElement>();
const manifoldOutliersRef = createRef<HTMLDivElement>();
const manifoldPeakRef = createRef<HTMLDivElement>();

const resonanceFooterRef = createRef<HTMLSpanElement>();
const resonanceTitleRef = createRef<HTMLSpanElement>();

/*
paintManifoldMeta paints pilot-wave canvas metadata from the current DRAW
manifold batch into the LiveManifoldMeta shell.
*/
export const paintManifoldMeta = (value: unknown, focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ManifoldFrame[];
	const manifold = rows
		.filter(
			(frame) => focusSymbol === "" || frame.symbol === focusSymbol,
		)
		.at(-1);
	const contour = terminalStore.state.fieldStyle === "Contour";
	const waiting = manifold === undefined;
	const display = terminalFluidDisplayLatticeFromFrame(manifold ?? null);
	const field = isFluidFieldMatrix(display) ? display : [];
	const { columns, rows: gridRows } = fluidGridDimensions(
		manifold ?? null,
		field,
	);
	const stats = terminalFluidFieldStats(field, contour);

	if (manifoldWaitingRef.current !== null) {
		manifoldWaitingRef.current.style.display = waiting ? "" : "none";
	}

	if (manifoldGridRef.current !== null) {
		manifoldGridRef.current.style.display = waiting ? "none" : "";
		manifoldGridRef.current.textContent = waiting
			? ""
			: `grid ${String(columns)}×${String(gridRows)}`;
	}

	if (manifoldOutliersRef.current !== null) {
		manifoldOutliersRef.current.style.display = waiting ? "none" : "";
		manifoldOutliersRef.current.textContent = waiting
			? ""
			: `outliers ${String(stats.outliers)}`;
	}

	if (manifoldPeakRef.current !== null) {
		manifoldPeakRef.current.style.display = waiting ? "none" : "";
		manifoldPeakRef.current.textContent = waiting
			? ""
			: `peak ${stats.peak.toFixed(2)}`;
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
		.filter(
			(frame) => focusSymbol === "" || frame.symbol === focusSymbol,
		)
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
		.filter(
			(frame) => focusSymbol === "" || frame.symbol === focusSymbol,
		)
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
		<div ref={manifoldOutliersRef} />
		<div ref={manifoldPeakRef} />
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
