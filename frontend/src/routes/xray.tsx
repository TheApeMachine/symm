import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import {
	DEFAULT_FOCUS_SYMBOL,
	appStore,
	focusStore,
	resonanceArtifactStore,
	symbolsStore,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { paintXrayHierarchy } from "#/components/terminal/xray-hierarchy";
import { paintXrayLatent } from "#/components/terminal/xray-latent";
import {
	XrayFactsPanel,
	XrayHawkesPanel,
	XrayHierarchyPanel,
	XrayLatentPanel,
	XrayManifoldPanel,
} from "#/components/terminal/xray-panels";
import {
	getAllRetainedResonance,
	retainResonanceRow,
} from "#/components/terminal/xray-view";

const XrayPaintBridge = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);

	useEffect(() => {
		const updatePaint = (state: typeof resonanceArtifactStore.state) => {
			const last = state.getLast();

			if (last) {
				const row = last.unpack() as unknown as Record<string, unknown>;
				const sym = typeof row.symbol === "string" ? row.symbol : "";

				if (sym) {
					retainResonanceRow(sym, row);
					appStore.actions.observeSymbols([sym]);
				}
			}

			const universe = getAllRetainedResonance();
			paintXrayHierarchy(universe, focusSymbol);
			paintXrayLatent(universe, focusSymbol);
		};

		updatePaint(resonanceArtifactStore.state);
		const subscription = resonanceArtifactStore.subscribe((state) => {
			updatePaint(state);
		});

		return () => {
			subscription.unsubscribe();
		};
	}, [focusSymbol]);

	return null;
};

const XrayCarrierBar = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const [symbols, setSymbols] = useState<string[]>(() => {
		const initial = new Set<string>(symbolsStore.state);
		for (const row of getAllRetainedResonance()) {
			if (row.symbol) initial.add(row.symbol as string);
		}
		if (initial.size === 0) initial.add(DEFAULT_FOCUS_SYMBOL);
		return [...initial];
	});

	useEffect(() => {
		const syncSymbols = () => {
			const current = new Set<string>(symbolsStore.state);
			for (const row of getAllRetainedResonance()) {
				if (row.symbol) current.add(row.symbol as string);
			}
			if (current.size === 0) current.add(DEFAULT_FOCUS_SYMBOL);
			const list = [...current];
			setSymbols((prev) => (prev.join(",") === list.join(",") ? prev : list));
		};

		syncSymbols();
		const sub1 = symbolsStore.subscribe(syncSymbols);
		const sub2 = resonanceArtifactStore.subscribe(syncSymbols);

		return () => {
			sub1.unsubscribe();
			sub2.unsubscribe();
		};
	}, []);

	return (
		<div className="flex h-11.5 shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
			<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				Inspect symbol
			</span>
			{symbols.map((sym) => {
				const active = sym === focusSymbol;
				return (
					<button
						key={sym}
						type="button"
						onClick={() => {
							appStore.actions.updateFocusSymbol(sym);
							terminalStore.actions.selectFocusSymbol(sym);
						}}
						className={`shrink-0 cursor-pointer rounded-[3px] border px-2.75 py-1 font-medium font-mono text-[11px] transition-colors ${
							active
								? "border-(--acc) bg-[color-mix(in_srgb,var(--acc)_14%,transparent)] text-(--acc)"
								: "border-(--line2) bg-transparent text-(--f3) hover:border-(--acc)"
						}`}
					>
						{sym}
					</button>
				);
			})}
		</div>
	);
};

const RouteComponent = () => (
	<div className="flex h-full min-w-275 flex-col">
		<XrayPaintBridge />
		<XrayCarrierBar />
		<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
			<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
				<XrayHierarchyPanel />
				<XrayHawkesPanel />
			</div>

			<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
				<div className="shrink-0 px-3.5 pt-3 pb-1.5">
					<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Latent manifold
					</div>
					<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
						universe embedding · clustered by regime · focus pulses
					</div>
				</div>
				<div className="relative mx-2 h-75 shrink-0">
					<XrayLatentPanel />
					<div className="pointer-events-none absolute bottom-1.5 left-2.5 font-mono text-[8.5px] text-(--f4)">
						latent-1 →
					</div>
					<div className="pointer-events-none absolute top-2.5 left-1.5 font-mono text-[8.5px] text-(--f4) [writing-mode:vertical-rl]">
						latent-2 →
					</div>
				</div>

				<XrayFactsPanel />
				<XrayManifoldPanel />
			</div>
		</div>
	</div>
);

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});