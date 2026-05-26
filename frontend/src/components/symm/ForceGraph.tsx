
import { memo, useCallback, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { CausalGraphDataProvider } from "#/components/symm/causal-graph-data-provider";
import { formatGraphCaption } from "#/components/symm/causal-graph-layout";
import { initCausalForceGraph } from "#/components/symm/force-graph-init";
import { Flex } from "#/components/ui/flex";
import "#/lib/symm/scichart-setup";

type ForceGraphProps = {
	symbol?: string;
	className?: string;
};

export const ForceGraph = memo(function ForceGraph({
	symbol = "BTC/EUR",
	className = "",
}: ForceGraphProps) {
	const [caption, setCaption] = useState("awaiting causal_graph…");

	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("initCausalForceGraph requires an HTMLDivElement root");
		}

		return initCausalForceGraph(rootElement);
	}, []);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof initCausalForceGraph>) => {
			const snapshot = CausalGraphDataProvider.snapshot(symbol);

			if (snapshot) {
				setCaption(formatGraphCaption(snapshot));
				result.controls.update(snapshot);
			}

			const unregister = CausalGraphDataProvider.registerSymbol(
				symbol,
				(row) => {
					setCaption(formatGraphCaption(row));
					result.controls.update(row);
				},
			);

			return () => {
				unregister();
				result.controls.dispose();
			};
		},
		[symbol],
	);

	return (
		<Flex.Column
			fullWidth
			fullHeight
			className={`min-h-0 overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<Flex.Row
				align="center"
				justify="between"
				padding={1}
				className="shrink-0 border-b border-(--dash-border)"
			>
				<span className="text-xs font-semibold tracking-wide">Causal DAG</span>
				<span className="text-[10px] text-(--dash-muted)">{symbol}</span>
			</Flex.Row>
			<span className="shrink-0 px-2 pb-1 text-[9px] text-(--dash-muted)">
				{caption}
			</span>
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="min-h-0 w-full flex-1"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
		</Flex.Column>
	);
});

export default ForceGraph;
