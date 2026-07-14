import { useRef } from "react";
import { appStore } from "#/collections/app";
import { manifoldStore } from "#/collections/manifold";
import { resonanceStore } from "#/collections/resonance";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

/*
LiveManifoldMeta paints fluid-density canvas metadata without React reconciliation.
*/
export const LiveManifoldMeta = ({ focusSymbol }: { focusSymbol: string }) => {
	const epochRef = useRef<HTMLDivElement>(null);
	const massRef = useRef<HTMLDivElement>(null);
	const modesRef = useRef<HTMLDivElement>(null);
	const waitingRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const manifold =
				manifoldStore.state.manifold[focusSymbol]?.values().at(-1) ?? null;
			const waiting = manifold === null;

			if (waitingRef.current !== null) {
				waitingRef.current.style.display = waiting ? "" : "none";
			}

			if (epochRef.current !== null) {
				epochRef.current.style.display = waiting ? "none" : "";
				epochRef.current.textContent = waiting
					? ""
					: `epoch ${String(manifold.epoch)}`;
			}

			if (massRef.current !== null) {
				massRef.current.style.display = waiting ? "none" : "";
				massRef.current.textContent = waiting
					? ""
					: `mass ${String(manifold.visibleMass)}`;
			}

			if (modesRef.current !== null) {
				modesRef.current.style.display = waiting ? "none" : "";
				modesRef.current.textContent = waiting
					? ""
					: `modes ${String(manifold.oscillatorCount)}`;
			}
		},
		[manifoldStore, appStore],
		[focusSymbol],
	);

	return (
		<div>
			<div ref={waitingRef}>waiting</div>
			<div ref={epochRef} />
			<div ref={massRef} />
			<div ref={modesRef} />
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
