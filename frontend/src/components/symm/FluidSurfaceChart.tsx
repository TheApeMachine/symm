import { memo, useCallback, useRef } from "react";
import { SciChartReact } from "scichart-react";

import type { FieldSnapshotEvent } from "#/lib/symm/events";
import {
	initFluidSurface,
	type FluidSurfaceInitResult,
} from "#/lib/symm/fluid-surface-controller";
import {
	registerFieldSnapshotListener,
	unregisterFieldSnapshotListener,
} from "#/lib/symm/feed";
import { formatFluidScalar } from "#/lib/symm/fluid-format";
import { useSymmConnected, useSymmFieldSnapshot } from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type FluidSurfaceChartProps = {
	className?: string;
};

/** Live 3D terrain of market turbulence (Reynolds) over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart({
	className = "",
}: FluidSurfaceChartProps) {
	const fieldListenerRef = useRef<
		((snapshot: FieldSnapshotEvent) => void) | null
	>(null);

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => initFluidSurface(rootElement),
		[],
	);

	const onInit = useCallback((result: FluidSurfaceInitResult) => {
		const listener = (snapshot: FieldSnapshotEvent) => {
			result.update(snapshot);
		};

		fieldListenerRef.current = listener;
		registerFieldSnapshotListener(listener);
	}, []);

	const onDelete = useCallback((result?: FluidSurfaceInitResult) => {
		if (fieldListenerRef.current) {
			unregisterFieldSnapshotListener(fieldListenerRef.current);
			fieldListenerRef.current = null;
		}

		result?.dispose();
	}, []);

	return (
		<div
			className={`flex h-full min-h-0 w-full flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<FluidSurfaceHeader />
			<div className="relative min-h-0 flex-1 overflow-hidden touch-none">
				<SciChartReact
					initChart={initChart}
					onInit={onInit}
					onDelete={onDelete}
					style={{ position: "absolute", height: "100%", width: "100%" }}
				/>
			</div>
			<p className="shrink-0 truncate border-t border-(--dash-border) px-2 py-0.5 text-[9px] text-(--dash-muted)">
				Reynolds · change rank × vol rank · drag to orbit · field σ = median/MAD
			</p>
		</div>
	);
});

const FluidSurfaceHeader = memo(function FluidSurfaceHeader() {
	const connected = useSymmConnected();
	const snapshot = useSymmFieldSnapshot();
	const field = snapshot?.field;
	const count = snapshot?.symbol_count ?? 0;

	return (
		<div className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-b border-(--dash-border) px-2 py-1.5">
			<span className="text-xs font-semibold tracking-wide">Market fluid</span>
			<span className="text-[10px] text-(--dash-muted)">
				{count > 0
					? `${count} sampled`
					: connected
						? "Warming — partial rows streaming"
						: "Offline"}
			</span>
			{field ? (
				<div className="ml-auto flex flex-wrap gap-3 text-[10px] tabular-nums text-(--dash-muted)">
					<FieldMetric label="Re" value={field.re} />
					<FieldMetric label="Vort" value={field.vort} />
					<FieldMetric label="Div" value={field.div} />
					<FieldMetric label="Turb" value={field.turb} />
				</div>
			) : null}
		</div>
	);
});

function FieldMetric({ label, value }: { label: string; value: number }) {
	return (
		<span>
			{label}{" "}
			<span className="font-medium text-(--dash-text)">
				{formatFluidScalar(value)}
			</span>
		</span>
	);
}
