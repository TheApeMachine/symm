import { type RefObject, useCallback, useState } from "react";
import type { SciChart3DSurface } from "scichart";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { initManifoldSurfaceChart } from "#/components/charts/manifold/init-manifold-surface-chart";
import {
	attachManifoldPush,
	detachManifoldPush,
	type ManifoldPushBridge,
} from "#/components/charts/manifold/manifold-push-bridge";
import { formatManifoldReading } from "#/components/charts/manifold/manifold-snapshot";
import type { ManifoldFieldSnapshot } from "#/components/charts/manifold/types";

const formatReading = (frame: ManifoldFieldSnapshot | null): string[] => {
	if (frame === null) {
		return ["Awaiting manifold field snapshot…"];
	}

	return formatManifoldReading(frame);
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
			<div className="pointer-events-none absolute left-3 top-3 rounded-md border border-white/10 bg-black/55 px-2.5 py-1.5 font-mono text-[9px] leading-4 text-white/90">
				<div className="mb-0.5 text-[8px] uppercase tracking-[0.16em] text-white/60">
					Manifold dynamics
				</div>
				{formatReading(snapshot).map((line) => (
					<div key={line}>{line}</div>
				))}
			</div>
		</div>
	);
};
