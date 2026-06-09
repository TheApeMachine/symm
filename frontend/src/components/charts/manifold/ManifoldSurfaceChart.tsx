import { type RefObject, useCallback, useState } from "react";
import type { SciChart3DSurface } from "scichart";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { initManifoldSurfaceChart } from "#/components/charts/manifold/init-manifold-surface-chart";
import {
	attachManifoldPush,
	detachManifoldPush,
	type ManifoldPushBridge,
} from "#/components/charts/manifold/manifold-push-bridge";
import type { ManifoldFieldSnapshot } from "#/components/charts/manifold/types";

const formatReading = (frame: ManifoldFieldSnapshot | null): string[] => {
	if (frame === null) {
		return ["Awaiting manifold field snapshot…"];
	}

	const reading = frame.reading;
	const whales = frame.carriers.filter(
		(carrier) => carrier.role === "whale",
	).length;
	const symbols = frame.carriers.filter(
		(carrier) => carrier.role === "symbol",
	).length;

	return [
		`∇p norm ${reading.pressure_grad_norm.toExponential(2)}`,
		`|Ψ|² ${reading.coherence_mag2.toExponential(2)}`,
		`guidance ${reading.guidance_speed.toExponential(2)}`,
		`viscosity ${reading.viscosity_proxy.toExponential(2)}`,
		`div ${reading.divergence.toExponential(2)}`,
		`carriers ${symbols} symbols · ${whales} whales`,
	];
};

export const ManifoldSurfaceChart = ({
	bridgeRef,
}: {
	bridgeRef: RefObject<ManifoldPushBridge>;
}) => {
	const [snapshot, setSnapshot] = useState<ManifoldFieldSnapshot | null>(null);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof initManifoldSurfaceChart>) => {
			const bridge = bridgeRef.current;

			if (!bridge) {
				return;
			}

			const push = (frame: ManifoldFieldSnapshot) => {
				result.controls.push(frame);
				setSnapshot(frame);
			};

			attachManifoldPush(bridge, push);

			return () => {
				detachManifoldPush(bridge);
			};
		},
		[bridgeRef],
	);

	return (
		<div className="relative h-full w-full">
			<SciChartReact<
				SciChart3DSurface,
				TResolvedReturnType<typeof initManifoldSurfaceChart>
			>
				initChart={initManifoldSurfaceChart}
				onInit={onInit}
				style={{ height: "100%", width: "100%" }}
			/>
			<div className="pointer-events-none absolute left-3 top-3 rounded-md border border-white/10 bg-black/55 px-3 py-2 font-mono text-[11px] leading-5 text-white/90">
				<div className="mb-1 text-[10px] uppercase tracking-[0.18em] text-white/60">
					Manifold dynamics
				</div>
				{formatReading(snapshot).map((line) => (
					<div key={line}>{line}</div>
				))}
			</div>
		</div>
	);
};
