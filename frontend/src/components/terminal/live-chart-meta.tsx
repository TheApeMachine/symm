import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

/*
LiveManifoldMeta paints pilot-wave canvas metadata without React reconciliation.
*/
export const LiveManifoldMeta = ({ focusSymbol }: { focusSymbol: string }) => {
	const gridRef = useRef<HTMLDivElement>(null);
	const outliersRef = useRef<HTMLDivElement>(null);
	const peakRef = useRef<HTMLDivElement>(null);
	const waitingRef = useRef<HTMLDivElement>(null);
	const online = useSelector(appStore, (state) => state.online);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);

	useDirectStorePaint(
		getWorker(),
		[{ store: "manifold", key: focusSymbol }],
		(buffers) => {
			const manifold = (buffers[`manifold:${focusSymbol}`] ?? []).at(
				-1,
			) as ManifoldFrame | undefined;
			const contour = fieldStyle === "Contour";
			const waiting = manifold === undefined;
			const display = terminalFluidDisplayLatticeFromFrame(manifold ?? null);
			const field = isFluidFieldMatrix(display) ? display : [];
			const { columns, rows } = fluidGridDimensions(manifold ?? null, field);
			const stats = terminalFluidFieldStats(field, contour);

			if (waitingRef.current !== null) {
				waitingRef.current.style.display = waiting ? "" : "none";
			}

			if (gridRef.current !== null) {
				gridRef.current.style.display = waiting ? "none" : "";
				gridRef.current.textContent = waiting
					? ""
					: `grid ${String(columns)}×${String(rows)}`;
			}

			if (outliersRef.current !== null) {
				outliersRef.current.style.display = waiting ? "none" : "";
				outliersRef.current.textContent = waiting
					? ""
					: `outliers ${String(stats.outliers)}`;
			}

			if (peakRef.current !== null) {
				peakRef.current.style.display = waiting ? "none" : "";
				peakRef.current.textContent = waiting
					? ""
					: `peak ${stats.peak.toFixed(2)}`;
			}
		},
		[online, focusSymbol, fieldStyle],
	);

	return (
		<div>
			<div ref={waitingRef}>waiting</div>
			<div ref={gridRef} />
			<div ref={outliersRef} />
			<div ref={peakRef} />
		</div>
	);
};

/*
LiveResonanceFooter paints predictive-coding footer metadata without React.
*/
export const LiveResonanceFooter = ({
	focusSymbol,
}: {
	focusSymbol: string;
}) => {
	const footerRef = useRef<HTMLSpanElement>(null);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[{ store: "resonance", key: focusSymbol }],
		(buffers) => {
			const resonance = (buffers[`resonance:${focusSymbol}`] ?? []).at(
				-1,
			) as ResonanceFrame | undefined;

			if (footerRef.current !== null) {
				footerRef.current.textContent =
					resonance === undefined
						? "waiting"
						: `symbol ${String(resonance.symbol)}`;
			}
		},
		[online, focusSymbol],
	);

	return <span ref={footerRef} />;
};

/*
LiveResonanceTitle paints predictive-coding title samples without React.
*/
export const LiveResonanceTitle = ({
	focusSymbol,
}: {
	focusSymbol: string;
}) => {
	const titleRef = useRef<HTMLSpanElement>(null);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[{ store: "resonance", key: focusSymbol }],
		(buffers) => {
			const resonance = (buffers[`resonance:${focusSymbol}`] ?? []).at(
				-1,
			) as ResonanceFrame | undefined;

			if (titleRef.current !== null) {
				titleRef.current.textContent =
					resonance === undefined
						? "waiting"
						: `${String(resonance.samples)} samples`;
			}
		},
		[online, focusSymbol],
	);

	return <span ref={titleRef} />;
};
