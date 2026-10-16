import {
	DEFAULT_FOCUS_SYMBOL,
	focusAtom,
	observeSymbols,
	resonanceStore,
	symbolsAtom,
	type RingBuffer,
} from "#/collections/app";
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
import { Button, Flex, Grid, Typography } from "#/components/ui";
import type { Value } from "#/types/value";
import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";

const XrayPaintBridge = () => {
	const focusSymbol = useSelector(focusAtom);

	useEffect(() => {
		const updatePaint = (state: Record<string, RingBuffer<Value>>) => {
			for (const ring of Object.values(state)) {
				const last = ring && !ring.isEmpty() ? ring.getLast() : null;

				if (!last) continue;

				const sym = typeof last.scope === "string" ? last.scope : "";

				if (sym) {
					retainResonanceRow(sym, last as unknown as Record<string, unknown>);
					observeSymbols([sym]);
				}
			}

			const universe = getAllRetainedResonance();
			paintXrayHierarchy(universe, focusSymbol);
			paintXrayLatent(universe, focusSymbol);
		};

		updatePaint(resonanceStore.state);
		const subscription = resonanceStore.subscribe((state) => {
			updatePaint(state);
		});

		return () => {
			subscription.unsubscribe();
		};
	}, [focusSymbol]);

	return null;
};

const XrayCarrierBar = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const [symbols, setSymbols] = useState<string[]>(() => {
		const initial = new Set<string>(symbolsAtom.get());
		for (const row of getAllRetainedResonance()) {
			if (typeof row.scope === "string") initial.add(row.scope);
		}
		if (initial.size === 0) initial.add(DEFAULT_FOCUS_SYMBOL);
		return [...initial];
	});

	useEffect(() => {
		const syncSymbols = () => {
			const current = new Set<string>(symbolsAtom.get());
			for (const row of getAllRetainedResonance()) {
				if (typeof row.scope === "string") current.add(row.scope);
			}
			if (current.size === 0) current.add(DEFAULT_FOCUS_SYMBOL);
			const list = [...current];
			setSymbols((prev) => (prev.join(",") === list.join(",") ? prev : list));
		};

		syncSymbols();
		const sub1 = symbolsAtom.subscribe(syncSymbols);
		const sub2 = resonanceStore.subscribe(syncSymbols);

		return () => {
			sub1.unsubscribe();
			sub2.unsubscribe();
		};
	}, []);

	return (
		<Flex.Row
			align="center"
			className="h-11.5 shrink-0 gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5"
		>
			<Typography.Label
				size="xxs"
				tone="f3"
				className="mr-1 shrink-0 uppercase tracking-[0.13em]"
			>
				Inspect symbol
			</Typography.Label>
			{symbols.map((sym) => {
				const active = sym === focusSymbol;
				return (
					<Button
						key={sym}
						variant={active ? "solid" : "quiet"}
						tone={active ? "accent" : "muted"}
						size="xs"
						onClick={() => {
							focusAtom.set(sym);
						}}
						className="shrink-0 font-mono text-[11px]"
					>
						{sym}
					</Button>
				);
			})}
		</Flex.Row>
	);
};

const RouteComponent = () => (
	<Flex.Column className="min-w-275" fullWidth fullHeight>
		<XrayPaintBridge />
		<XrayCarrierBar />
		<Grid className="min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
			<Flex.Column className="min-h-0 overflow-auto border-(--line) border-r">
				<XrayHierarchyPanel />
				<XrayHawkesPanel />
			</Flex.Column>

			<Flex.Column className="min-h-0 overflow-auto bg-(--surface)">
				<div className="shrink-0 px-3.5 pt-3 pb-1.5">
					<Typography.Label
						size="xxs"
						tone="f3"
						className="font-semibold uppercase tracking-[0.13em]"
					>
						Latent manifold
					</Typography.Label>
					<Typography.Paragraph className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
						universe embedding · clustered by regime · focus pulses
					</Typography.Paragraph>
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
			</Flex.Column>
		</Grid>
	</Flex.Column>
);

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});
