import { memo, useCallback } from "react";
import { SciChartReact } from "scichart-react";

import {
	initFluidSurface,
	type FluidSurfaceInitResult,
} from "#/lib/symm/fluid-surface-controller";
import { registerFluidSurface, unregisterFluidSurface } from "#/lib/symm/feed";
import { summarizeFluidScaling } from "#/lib/symm/fluid-grid";
import { useSymmConnected, useSymmFieldSnapshot } from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type FluidSurfaceChartProps = {
	className?: string;
};

/** Live 3D terrain of market turbulence (Reynolds) over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart({
	className = "",
}: FluidSurfaceChartProps) {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => initFluidSurface(rootElement),
		[],
	);

	const onInit = useCallback((result: FluidSurfaceInitResult) => {
		registerFluidSurface((snapshot) => result.update(snapshot));
	}, []);

	const onDelete = useCallback((result?: FluidSurfaceInitResult) => {
		result?.dispose();
		unregisterFluidSurface();
	}, []);

	return (
		<div
			className={`relative isolate flex h-full min-h-0 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<FluidSurfaceHeader />
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				onDelete={onDelete}
				className="min-h-0 w-full flex-1"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
			<p className="shrink-0 truncate border-t border-(--dash-border) px-2 py-0.5 text-[9px] text-(--dash-muted)">
				Reynolds · change rank × vol rank · drag to orbit
			</p>
		</div>
	);
});

const FluidSurfaceHeader = memo(function FluidSurfaceHeader() {
	const connected = useSymmConnected();
	const snapshot = useSymmFieldSnapshot();
	const field = snapshot?.field;
	const count = snapshot?.symbol_count ?? 0;
	const scaling = snapshot
		? summarizeFluidScaling(snapshot.symbols ?? [])
		: undefined;

	return (
		<div className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-b border-(--dash-border) px-2 py-1.5">
			<span className="text-xs font-semibold tracking-wide">Market fluid</span>
			<span className="text-[10px] text-(--dash-muted)">
				{count > 0
					? `${count} symbols`
					: connected
						? "Awaiting rescore…"
						: "Offline"}
			</span>
			{field ? (
				<div className="ml-auto flex flex-wrap gap-3 text-[10px] tabular-nums text-(--dash-muted)">
					<FieldMetric label="Re" value={field.re} />
					<FieldMetric label="Vort" value={field.vort} />
					<FieldMetric label="Div" value={field.div} />
					<FieldMetric label="Turb" value={field.turb} />
					{scaling && scaling.clippedCount > 0 ? (
						<span title={`Raw max ${scaling.rawMax.toFixed(3)}`}>
							Clipped {scaling.clippedCount} above p95{" "}
							<span className="font-medium text-(--dash-text)">
								{scaling.clippedAt.toFixed(3)}
							</span>
							{scaling.rawMaxSymbol ? ` · max ${scaling.rawMaxSymbol}` : ""}
						</span>
					) : null}
				</div>
			) : null}
		</div>
	);
});

function FieldMetric({ label, value }: { label: string; value: number }) {
	return (
		<span>
			{label}{" "}
			<span className="font-medium text-(--dash-text)">{value.toFixed(3)}</span>
		</span>
	);
}
