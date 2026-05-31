import { memo, useCallback, useMemo } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import type { SciChart3DSurface } from "scichart";

import { drawGenericSurface } from "#/components/symm/init-generic-surface-chart";
import type { LayoutPanel } from "#/lib/symm/layout-schema";
import {
	readHeightMatrix,
	StreamDataProvider,
} from "#/lib/symm/stream-data-provider";
import { gridFromPayload, gridFromSnapshot } from "#/lib/symm/fluid-grid";
import {
	isFieldSnapshotEvent,
	type FieldSnapshotEvent,
} from "#/lib/symm/events";
import "#/lib/symm/scichart-setup";

const heightsFromPayload = (
	payload: unknown,
	heightKey: string,
): number[][] | undefined => {
	const direct = readHeightMatrix(payload, heightKey);

	if (direct !== undefined) {
		return direct;
	}

	if (!isFieldSnapshotEvent(payload)) {
		return undefined;
	}

	const snapshot = payload as FieldSnapshotEvent;
	const grid = snapshot.grid?.heights?.length
		? gridFromPayload(snapshot.grid)
		: gridFromSnapshot(snapshot);

	return grid.heights;
};

export const GenericSurfaceChart = memo(function GenericSurfaceChart({
	panel,
}: {
	panel: LayoutPanel;
}) {
	const stream = panel.stream ?? "field_snapshot";
	const heightKey = panel.height_key ?? "grid.heights";

	const onInit = useCallback(
		(initResult: TResolvedReturnType<typeof drawGenericSurface>) => {
			const applyPayload = (payload: unknown) => {
				const heights = heightsFromPayload(payload, heightKey);

				if (heights === undefined) {
					return;
				}

				initResult.controls.update(heights);
			};

			const latest = StreamDataProvider.snapshot(stream);
			applyPayload(latest);

			const unregister = StreamDataProvider.subscribe(stream, applyPayload);

			return () => {
				unregister();
				initResult.controls.dispose();
			};
		},
		[heightKey, stream],
	);

	const initChart = useMemo(() => drawGenericSurface, []);

	return (
		<SciChartReact<
			SciChart3DSurface,
			TResolvedReturnType<typeof drawGenericSurface>
		>
			initChart={initChart}
			onInit={onInit}
			className="h-full w-full"
		/>
	);
});
