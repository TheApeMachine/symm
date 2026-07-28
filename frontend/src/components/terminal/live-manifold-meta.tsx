import { createRef } from "react";
import type { ManifoldFrame } from "#/collections/types";
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

const formatFieldMaximum = (value: number): string =>
	new Intl.NumberFormat("en", {
		maximumSignificantDigits: 3,
		notation: "scientific",
	}).format(value);

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

export const paintManifoldMeta = (value: unknown, focusSymbol: string) => {
	lastManifoldMetaBatch = value;
	paintManifoldMetaCompose(value, focusSymbol);
};

export const repaintManifoldMeta = (focusSymbol: string) => {
	if (lastManifoldMetaBatch === null) {
		return;
	}

	paintManifoldMetaCompose(lastManifoldMetaBatch, focusSymbol);
};

export const LiveManifoldMeta = () => (
	<div>
		<div ref={manifoldWaitingRef}>waiting</div>
		<div ref={manifoldGridRef} />
		<div ref={manifoldPopulationRef} />
		<div ref={manifoldProjectionRef} />
		<div ref={manifoldCoherenceRef} />
		<div ref={manifoldGasRef} />
	</div>
);
