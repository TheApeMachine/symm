import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo } from "react";
import { atoms, DEFAULT_FOCUS_SYMBOL, signals } from "#/collections/app";
import { paintXrayLatent } from "#/components/terminal/xray-latent";
import {
	XrayFactsPanel,
	XrayHawkesPanel,
	XrayHierarchyPanel,
	XrayLatentPanel,
	XrayManifoldPanel,
} from "#/components/terminal/xray-panels";
import { Button, Flex, Grid, Typography } from "#/components/ui";
import { resonanceUniverseRows } from "#/lib/resonance";

const XrayPaintBridge = () => {
	const focusSymbol = useSelector(atoms.focus);

	useEffect(() => {
		const updatePaint = () => {
			const universe = resonanceUniverseRows(signals.resonance.state);
			paintXrayLatent(universe, focusSymbol);
		};

		updatePaint();
		const resonanceSub = signals.resonance.subscribe(updatePaint);

		return () => {
			resonanceSub.unsubscribe();
		};
	}, [focusSymbol]);

	return null;
};

const XrayCarrierBar = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const resonanceKeys = useSelector(signals.resonance, (state) =>
		Object.keys(state),
	);
	const catalog = useSelector(atoms.symbols, (state) => state);

	const symbols = useMemo(() => {
		const set = new Set<string>(catalog);
		for (const key of resonanceKeys) {
			set.add(key);
		}
		if (set.size === 0) {
			set.add(DEFAULT_FOCUS_SYMBOL);
		}
		return [...set].sort((a, b) => a.localeCompare(b));
	}, [catalog, resonanceKeys]);

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
							atoms.focus.set(sym);
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
		<Grid
			fullWidth
			responsive={false}
			className="min-h-0 min-w-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]"
		>
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
