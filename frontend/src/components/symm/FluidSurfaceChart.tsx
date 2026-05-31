import { memo, useCallback, useMemo, useRef, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import type { SciChart3DSurface } from "scichart";

import { FluidDataProvider } from "#/components/symm/fluid-data-provider";
import { FluidVisualEditor } from "#/components/symm/FluidVisualEditor";
import {
	createDrawExample,
	type FluidSurfaceControls,
} from "#/components/symm/init-fluid-surface-chart";
import { Flex } from "#/components/ui/flex";
import {
	defaultFluidVisualParams,
	loadFluidVisualParams,
	saveFluidVisualParams,
	type FluidVisualParamKey,
	type FluidVisualParams,
} from "#/lib/symm/fluid-visual-params";
import "#/lib/symm/scichart-setup";

/** Live 3D surface over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart() {
	const [editMode, setEditMode] = useState(false);
	const [visualParams, setVisualParams] = useState(loadFluidVisualParams);
	const controlsRef = useRef<FluidSurfaceControls | null>(null);
	const visualParamsRef = useRef(visualParams);
	visualParamsRef.current = visualParams;

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
			const next = { ...visualParamsRef.current, [key]: value };
			setVisualParams(next);
			saveFluidVisualParams(next);
			controlsRef.current?.applyVisualParams(next);
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
		<Flex.Column className="fluid-chart-shell" fullHeight fullWidth>
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
