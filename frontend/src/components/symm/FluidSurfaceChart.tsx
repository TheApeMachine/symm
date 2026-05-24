import { memo, useCallback, useEffect, useRef, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import {
	drawFluidSurface,
	type FluidSurfaceControls,
} from "#/components/symm/draw-fluid-surface";
import { setFluidDisplay } from "#/lib/symm/feed";
import { formatFluidScalar, headerFieldMetrics } from "#/lib/symm/fluid-format";
import {
	useSymmConnected,
	useSymmFieldSnapshot,
	useSymmFluidDisplay,
	useSymmScanProgress,
} from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type FluidSurfaceChartProps = {
	className?: string;
};

/** Live 3D terrain of market turbulence (Reynolds) over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart({
	className = "",
}: FluidSurfaceChartProps) {
	const controlsRef = useRef<FluidSurfaceControls | null>(null);
	const snapshot = useSymmFieldSnapshot();

	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("drawFluidSurface requires an HTMLDivElement root");
		}

		return drawFluidSurface(rootElement);
	}, []);

	const onInit = useCallback(
		(initResult: TResolvedReturnType<typeof drawFluidSurface>) => {
			controlsRef.current = initResult.controls;

			return () => {
				initResult.controls.dispose();
				controlsRef.current = null;
			};
		},
		[],
	);

	useEffect(() => {
		if (snapshot && controlsRef.current) {
			controlsRef.current.update(snapshot);
		}
	}, [snapshot]);

	return (
		<div
			className={`flex h-full min-h-0 w-full flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<FluidSurfaceHeader />
			<div className="relative min-h-0 flex-1 overflow-hidden touch-none">
				<SciChartReact
					initChart={initChart}
					onInit={onInit}
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
	const scan = useSymmScanProgress();
	const display = useSymmFluidDisplay();
	const field = headerFieldMetrics(snapshot?.field, snapshot?.symbols);
	const count = Math.max(snapshot?.symbol_count ?? 0, scan.fluidSampled);
	const [emaAlpha, setEmaAlpha] = useState(display?.height_ema_alpha ?? 0.35);

	useEffect(() => {
		if (display?.height_ema_alpha !== undefined) {
			setEmaAlpha(display.height_ema_alpha);
		}
	}, [display?.height_ema_alpha]);

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
			<label className="flex items-center gap-2 text-[10px] text-(--dash-muted)">
				<span>EMA</span>
				<input
					type="range"
					min={0.05}
					max={1}
					step={0.05}
					value={emaAlpha}
					disabled={!connected}
					onChange={(event) => {
						const next = Number.parseFloat(event.target.value);

						if (!Number.isFinite(next)) {
							return;
						}

						setEmaAlpha(next);
						setFluidDisplay({ height_ema_alpha: next });
					}}
					className="h-1 w-24 accent-sky-400"
				/>
				<span className="w-8 tabular-nums text-(--dash-text)">
					{emaAlpha.toFixed(2)}
				</span>
			</label>
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
