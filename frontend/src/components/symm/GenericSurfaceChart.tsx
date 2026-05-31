import { memo, useCallback, useMemo, useRef, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import type { SciChart3DSurface } from "scichart";

import { FluidDataProvider } from "#/components/symm/fluid-data-provider";
import { FluidVisualEditor } from "#/components/symm/FluidVisualEditor";
import {
	createDrawExample,
	type FluidSurfaceControls,
} from "#/components/symm/init-fluid-surface-chart";
import { drawGenericSurface } from "#/components/symm/init-generic-surface-chart";
import type { LayoutPanel } from "#/lib/symm/layout-schema";
import {
	defaultFluidVisualParams,
	loadFluidVisualParams,
	saveFluidVisualParams,
	type FluidVisualParamKey,
	type FluidVisualParams,
} from "#/lib/symm/fluid-visual-params";
import {
	readHeightMatrix,
	StreamDataProvider,
} from "#/lib/symm/stream-data-provider";
import { gridFromPayload, gridFromSnapshot } from "#/lib/symm/fluid-grid";
import {
	isFieldSnapshotEvent,
	type FieldSnapshotEvent,
} from "#/lib/symm/events";
import { Flex } from "#/components/ui/flex";
import "#/lib/symm/scichart-setup";

const FLUID_STREAM = "field_snapshot";

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

const FluidFieldSurfaceChart = memo(function FluidFieldSurfaceChart() {
	const [editMode, setEditMode] = useState(false);
	const [visualParams, setVisualParams] = useState(loadFluidVisualParams);
	const controlsRef = useRef<FluidSurfaceControls | null>(null);

	const initChart = useMemo(
		() => createDrawExample(loadFluidVisualParams()),
		[],
	);

	const applyParams = useCallback((params: FluidVisualParams) => {
		setVisualParams(params);
		saveFluidVisualParams(params);
		controlsRef.current?.applyVisualParams(params);
	}, []);

	const handleParamChange = useCallback(
		(key: FluidVisualParamKey, value: number) => {
			setVisualParams((previous) => {
				const next = { ...previous, [key]: value };
				saveFluidVisualParams(next);
				controlsRef.current?.applyVisualParams(next);

				return next;
			});
		},
		[],
	);

	const handleReset = useCallback(() => {
		applyParams(defaultFluidVisualParams());
	}, [applyParams]);

	const onInit = useCallback(
		(initResult: TResolvedReturnType<ReturnType<typeof createDrawExample>>) => {
			controlsRef.current = initResult.controls;
			initResult.controls.applyVisualParams(loadFluidVisualParams());

			const unregister = FluidDataProvider.registerSink(
				initResult.controls.update,
			);

			return () => {
				unregister();
				controlsRef.current = null;
				initResult.controls.dispose();
			};
		},
		[],
	);

	return (
		<Flex.Column className="fluid-chart-shell relative h-full w-full min-h-0">
			<Flex.Row className="absolute top-2 left-2 z-20">
				<button
					type="button"
					className={`rounded border px-2 py-0.5 text-[10px] uppercase tracking-wide ${
						editMode
							? "border-(--dash-accent) bg-(--dash-row-active) text-(--dash-accent)"
							: "border-(--dash-border) bg-(--dash-panel)/90 text-(--dash-muted) hover:text-(--dash-text)"
					}`}
					onClick={() => setEditMode((active) => !active)}
				>
					{editMode ? "Done" : "Edit"}
				</button>
			</Flex.Row>
			{editMode ? (
				<FluidVisualEditor
					params={visualParams}
					onChange={handleParamChange}
					onReset={handleReset}
				/>
			) : null}
			<SciChartReact<
				SciChart3DSurface,
				TResolvedReturnType<ReturnType<typeof createDrawExample>>
			>
				initChart={initChart}
				onInit={onInit}
				className="h-full w-full min-h-0"
			/>
		</Flex.Column>
	);
});

const GenericStreamSurfaceChart = memo(function GenericStreamSurfaceChart({
	panel,
}: {
	panel: LayoutPanel;
}) {
	const stream = panel.stream ?? FLUID_STREAM;
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

/*
GenericSurfaceChart renders schema-driven surface panels. The fluid field stream
accumulates incremental field_row frames in FluidDataProvider before projection.
*/
export const GenericSurfaceChart = memo(function GenericSurfaceChart({
	panel,
}: {
	panel: LayoutPanel;
}) {
	const stream = panel.stream ?? FLUID_STREAM;

	if (stream === FLUID_STREAM) {
		return <FluidFieldSurfaceChart />;
	}

	return <GenericStreamSurfaceChart panel={panel} />;
});
