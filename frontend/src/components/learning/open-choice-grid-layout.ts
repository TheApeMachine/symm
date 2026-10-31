import {
	applyOpacityToHtmlColor,
	ChartModifierBase2D,
	configure2DSurface,
	DoubleAnimator,
	easing,
	ESubSurfacePositionCoordinateMode,
	GenericAnimation,
	type I2DSubSurfaceOptions,
	type ISciChart2DDefinition,
	Rect,
	SciChartSubSurface,
	type TAxisDefinition,
	Thickness,
	type TModifierDefinition,
	type TSeriesDefinition,
	type TXywhCoordinates,
} from "scichart";

const TITLE = "#f4efe5";
const BORDER = "rgba(232, 163, 61, 0.45)";

/** Converts a multi-series surface into an animated per-series grid and back. */
export class GridLayoutModifier extends ChartModifierBase2D {
	public readonly type = "GridLayoutModifier";
	public isGridProperty = false;
	public columns = 2;

	public get isGrid() {
		return this.isGridProperty;
	}

	public set isGrid(value: boolean) {
		if (this.isGridProperty === value) return;

		this.isGridProperty = value;
		if (value) {
			this.makeGridLayout();
		} else {
			this.makeSingleChart();
		}
	}

	private makeSubChart(
		surfaceDef: ISciChart2DDefinition,
		i: number,
		width: number,
		height: number,
	) {
		const rs = this.parentSurface.renderableSeries.get(i);
		const rsDef = (surfaceDef.series as TSeriesDefinition[])[i];
		// Sub-surface rebuilds its own empty series; we reattach the live dataSeries below.
		// @ts-expect-error Builder series defs always carry xyData; strip it so we share the parent series.
		rsDef.xyData = {};

		const row = Math.floor(i / this.columns);
		const col = i % this.columns;
		const position = new Rect(col * width, row * height, width, height);
		const subChartOptions: I2DSubSurfaceOptions = {
			id: `subChart-${i}`,
			position: new Rect(0, 0, 1, 1),
			coordinateMode: ESubSurfacePositionCoordinateMode.Relative,
			padding: Thickness.fromNumber(3),
			title: rs.dataSeries?.dataSeriesName,
			titleStyle: {
				placeWithinChart: true,
				fontSize: 12,
				padding: Thickness.fromString("10 4 0 4"),
				color: TITLE,
			},
		};

		const subChart = SciChartSubSurface.createSubSurface(
			this.parentSurface,
			subChartOptions,
		);
		const xAxisDef = (surfaceDef.xAxes as TAxisDefinition[]).find(
			(a) => a.options?.id === rs.xAxisId,
		);
		const yAxisDef = (surfaceDef.yAxes as TAxisDefinition[]).find(
			(a) => a.options?.id === rs.yAxisId,
		);
		const modifiers = (surfaceDef.modifiers as TModifierDefinition[]).filter(
			(m) => m.type.toString() !== "GridLayoutModifier",
		);
		const subSurfaceDef: ISciChart2DDefinition = {
			xAxes: xAxisDef,
			yAxes: yAxisDef,
			series: rsDef,
			modifiers,
		};

		configure2DSurface(
			subSurfaceDef,
			subChart,
			this.parentSurface.webAssemblyContext2D,
		);
		subChart.renderableSeries.get(0).dataSeries = rs.dataSeries;
		rs.isVisible = false;

		const xAxis = subChart.xAxes.get(0);
		const yAxis = subChart.yAxes.get(0);
		xAxis.drawMinorGridLines = false;
		yAxis.drawMinorGridLines = false;
		const labelColor = xAxis.labelStyle.color ?? TITLE;

		const animation = new GenericAnimation<Rect>({
			from: new Rect(0, 0, 1, 1),
			to: position,
			onAnimate: (from, to, progress) => {
				const x = DoubleAnimator.interpolate(from.x, to.x, progress);
				const y = DoubleAnimator.interpolate(from.y, to.y, progress);
				const w = DoubleAnimator.interpolate(from.width, to.width, progress);
				const h = DoubleAnimator.interpolate(from.height, to.height, progress);
				(
					subChart as typeof subChart & {
						viewportBorder: { color: string; border: number };
					}
				).viewportBorder = {
					color: `rgba(232, 163, 61, ${progress * 0.5})`,
					border: 2,
				};
				xAxis.labelStyle = {
					color: applyOpacityToHtmlColor(labelColor, progress),
				};
				yAxis.labelStyle = {
					color: applyOpacityToHtmlColor(labelColor, progress),
				};
				yAxis.drawMinorGridLines = false;
				subChart.titleStyle = {
					color: applyOpacityToHtmlColor(TITLE, progress),
				};
				subChart.subPosition = new Rect(x, y, w, h);
			},
			duration: 500,
			ease: easing.outCubic,
		});
		subChart.addAnimation(animation);
	}

	private makeGridLayout() {
		const series = this.parentSurface.renderableSeries.asArray();
		const chartCount = series.length;
		if (chartCount === 0) return;

		const rows = Math.ceil(chartCount / this.columns);
		const width = 1 / this.columns;
		const height = 1 / rows;
		const surfaceDef = this.parentSurface.toJSON(true);

		for (let i = 0; i < chartCount; i++) {
			this.makeSubChart(surfaceDef, i, width, height);
		}

		for (const x of this.parentSurface.xAxes.asArray()) {
			x.isVisible = false;
		}
		for (const y of this.parentSurface.yAxes.asArray()) {
			y.isVisible = false;
		}
	}

	private makeSingleChart() {
		const count = this.parentSurface.subCharts.length;

		for (let i = 0; i < count; i++) {
			const subChart = this.parentSurface.subCharts[i];
			const xAxis = subChart.xAxes.get(0);
			const yAxis = subChart.yAxes.get(0);
			const labelColor = xAxis.labelStyle.color ?? TITLE;
			const seriesIndex = i;

			const animation = new GenericAnimation<TXywhCoordinates>({
				from: subChart.subPosition as TXywhCoordinates,
				to: new Rect(0, 0, 1, 1),
				onAnimate: (from, to, progress) => {
					const x = DoubleAnimator.interpolate(from.x, to.x, progress);
					const y = DoubleAnimator.interpolate(from.y, to.y, progress);
					const w = DoubleAnimator.interpolate(from.width, to.width, progress);
					const h = DoubleAnimator.interpolate(
						from.height,
						to.height,
						progress,
					);
					xAxis.labelStyle = {
						color: applyOpacityToHtmlColor(labelColor, 1 - progress * 0.99),
					};
					yAxis.labelStyle = {
						color: applyOpacityToHtmlColor(labelColor, 1 - progress * 0.99),
					};
					subChart.titleStyle = {
						color: applyOpacityToHtmlColor(TITLE, 1 - progress * 0.99),
					};
					(
						subChart as typeof subChart & {
							viewportBorder: { color: string; border: number };
						}
					).viewportBorder = {
						color: BORDER,
						border: 2,
					};
					subChart.subPosition = new Rect(x, y, w, h);
				},
				duration: 500,
				ease: easing.outCubic,
				onCompleted: () => {
					subChart.isVisible = false;
					const rs = subChart.renderableSeries.get(0);
					// Detach before delete so the shared parent dataSeries survives.
					(rs as { dataSeries?: unknown }).dataSeries = undefined;
					this.parentSurface.removeSubChart(subChart);
					this.parentSurface.renderableSeries.get(seriesIndex).isVisible = true;
				},
			});
			this.parentSurface.addAnimation(animation);
		}

		for (const x of this.parentSurface.xAxes.asArray()) {
			x.isVisible = true;
		}
		for (const y of this.parentSurface.yAxes.asArray()) {
			y.isVisible = true;
		}
	}
}
