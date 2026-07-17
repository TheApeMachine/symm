import { useRef } from "react";
import { manifoldStore } from "#/collections/manifold";
import { resonanceStore } from "#/collections/resonance";
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

/*
LiveManifoldMeta paints pilot-wave canvas metadata without React reconciliation.
*/
export const LiveManifoldMeta = ({ focusSymbol }: { focusSymbol: string }) => {
	const gridRef = useRef<HTMLDivElement>(null);
	const outliersRef = useRef<HTMLDivElement>(null);
	const peakRef = useRef<HTMLDivElement>(null);
	const waitingRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const manifold =
				manifoldStore.state.manifold[focusSymbol]?.values().at(-1) ?? null;
			const contour = terminalStore.state.fieldStyle === "Contour";
			const waiting = manifold === null;
			const display = terminalFluidDisplayLatticeFromFrame(manifold);
			const field = isFluidFieldMatrix(display) ? display : [];
			const { columns, rows } = fluidGridDimensions(manifold, field);
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
		[manifoldStore, terminalStore],
		[focusSymbol],
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

	useDirectStorePaint(
		() => {
			const resonance =
				resonanceStore.state.resonance[focusSymbol]?.values().at(-1) ?? null;

			if (footerRef.current !== null) {
				footerRef.current.textContent =
					resonance === null ? "waiting" : `symbol ${String(resonance.symbol)}`;
			}
		},
		[resonanceStore],
		[focusSymbol],
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

	useDirectStorePaint(
		() => {
			const resonance =
				resonanceStore.state.resonance[focusSymbol]?.values().at(-1) ?? null;

			if (titleRef.current !== null) {
				titleRef.current.textContent =
					resonance === null
						? "waiting"
						: `${String(resonance.samples)} samples`;
			}
		},
		[resonanceStore],
		[focusSymbol],
	);

	return <span ref={titleRef} />;
};
