import {
	applyOpacityToHtmlColor,
	ChartModifierBase2D,
	configure2DSurface,
	DoubleAnimator,
	easing,
	EHorizontalAnchorPoint,
	ESubSurfacePositionCoordinateMode,
	EVerticalAnchorPoint,
	FastBandRenderableSeries,
	FastLineRenderableSeries,
	GenericAnimation,
	type I2DSubSurfaceOptions,
	type ISciChart2DDefinition,
	type IRenderableSeries,
	NativeTextAnnotation,
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

	private seriesName(rs: { dataSeries?: { dataSeriesName?: string } | null }) {
		return String(rs.dataSeries?.dataSeriesName ?? "");
	}

	private lotIdFromSeriesName(name: string) {
		return name.replace(/::(shadow|stop-band|shadow-band)$/, "");
	}

	private isCompanion(rs: { dataSeries?: { dataSeriesName?: string } | null }) {
		const name = this.seriesName(rs);
		return (
			name.includes("::shadow") ||
			name.includes("::stop-band") ||
			name.includes("::shadow-band")
		);
	}

	private findByName(name: string) {
		return this.parentSurface.renderableSeries
			.asArray()
			.find((item) => this.seriesName(item) === name);
	}

	private attachCompanionLine(
		subChart: SciChartSubSurface,
		source: IRenderableSeries | undefined,
		dashed: boolean,
	) {
		if (!source?.dataSeries) return;

		subChart.renderableSeries.add(
			new FastLineRenderableSeries(this.parentSurface.webAssemblyContext2D, {
				dataSeries: source.dataSeries,
				stroke: source.stroke,
				strokeThickness: source.strokeThickness,
				strokeDashArray: dashed ? [6, 4] : undefined,
			}),
		);
		source.isVisible = false;
	}

	private attachCompanionBand(
		subChart: SciChartSubSurface,
		source: IRenderableSeries | undefined,
		dashed: boolean,
		atIndex: number,
	) {
		if (!source?.dataSeries) return;

		const band = source as FastBandRenderableSeries;
		// Insert under the mark path — never remove/re-add a series that shares
		// a live dataSeries; SciChart dispose would wipe parent capacity.
		subChart.renderableSeries.insert(
			atIndex,
			new FastBandRenderableSeries(this.parentSurface.webAssemblyContext2D, {
				dataSeries: source.dataSeries,
				stroke: "#00000000",
				strokeY1: band.strokeY1 || source.stroke,
				strokeThickness: 1,
				strokeY1DashArray: dashed ? [4, 3] : undefined,
				fill: band.fill || "rgba(255,255,255,0.12)",
				fillY1: band.fillY1 || band.fill || "rgba(255,255,255,0.12)",
			}),
		);
		source.isVisible = false;
	}

	private attachTip(subChart: SciChartSubSurface, lotId: string) {
		const tip = this.parentSurface.annotations
			.asArray()
			.find((annotation) => annotation.id === `tip:${lotId}`) as
			| NativeTextAnnotation
			| undefined;

		if (!tip) return;

		subChart.annotations.add(
			new NativeTextAnnotation({
				id: `tip:${lotId}`,
				x1: tip.x1,
				y1: tip.y1,
				text: tip.text,
				fontSize: tip.fontSize,
				fontFamily: tip.fontFamily,
				textColor: tip.textColor,
				horizontalAnchorPoint: EHorizontalAnchorPoint.Right,
				verticalAnchorPoint: EVerticalAnchorPoint.Bottom,
			}),
		);
		tip.isHidden = true;
	}

	private makeSubChart(
		surfaceDef: ISciChart2DDefinition,
		rsIndex: number,
		panelIndex: number,
		width: number,
		height: number,
	) {
		const rs = this.parentSurface.renderableSeries.get(rsIndex);
		const rsDef = (surfaceDef.series as TSeriesDefinition[])[rsIndex];
		// Sub-surface rebuilds its own empty series; we reattach the live dataSeries below.
		// @ts-expect-error Builder series defs always carry xyData; strip it so we share the parent series.
		rsDef.xyData = {};

		const row = Math.floor(panelIndex / this.columns);
		const col = panelIndex % this.columns;
		const position = new Rect(col * width, row * height, width, height);
		const lotId = this.lotIdFromSeriesName(this.seriesName(rs));
		const subChartOptions: I2DSubSurfaceOptions = {
			id: `subChart-${panelIndex}`,
			position: new Rect(0, 0, 1, 1),
			coordinateMode: ESubSurfacePositionCoordinateMode.Relative,
			padding: Thickness.fromNumber(3),
			title: lotId,
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

		const primary = subChart.renderableSeries.get(0);
		primary.dataSeries = rs.dataSeries;
		rs.isVisible = false;

		// Insert floor bands beneath the mark path (index 0 / 1). Do not remove
		// `primary` — that disposes the shared dataSeries and crashes on capacity.
		this.attachCompanionBand(
			subChart,
			this.findByName(`${lotId}::stop-band`),
			false,
			0,
		);
		this.attachCompanionBand(
			subChart,
			this.findByName(`${lotId}::shadow-band`),
			true,
			subChart.renderableSeries.size() > 1 ? 1 : 0,
		);
		this.attachCompanionLine(
			subChart,
			this.findByName(`${lotId}::shadow`),
			true,
		);
		this.attachTip(subChart, lotId);

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
		const all = this.parentSurface.renderableSeries.asArray();
		const primaryIndexes = all
			.map((rs, index) => ({ rs, index }))
			.filter(({ rs }) => !this.isCompanion(rs))
			.map(({ index }) => index);
		const chartCount = primaryIndexes.length;
		if (chartCount === 0) return;

		const rows = Math.ceil(chartCount / this.columns);
		const width = 1 / this.columns;
		const height = 1 / rows;
		const surfaceDef = this.parentSurface.toJSON(true);

		for (let panel = 0; panel < chartCount; panel++) {
			this.makeSubChart(surfaceDef, primaryIndexes[panel], panel, width, height);
		}

		for (const rs of all) {
			if (this.isCompanion(rs)) {
				rs.isVisible = false;
			}
		}

		for (const x of this.parentSurface.xAxes.asArray()) {
			x.isVisible = false;
		}
		for (const y of this.parentSurface.yAxes.asArray()) {
			y.isVisible = false;
		}
	}

	private restoreLot(lotId: string) {
		for (const rs of this.parentSurface.renderableSeries.asArray()) {
			const name = this.seriesName(rs);
			if (name === lotId || name.startsWith(`${lotId}::`)) {
				rs.isVisible = true;
			}
		}

		const tip = this.parentSurface.annotations
			.asArray()
			.find((annotation) => annotation.id === `tip:${lotId}`);
		if (tip) tip.isHidden = false;
	}

	private makeSingleChart() {
		const count = this.parentSurface.subCharts.length;

		for (let i = 0; i < count; i++) {
			const subChart = this.parentSurface.subCharts[i];
			const xAxis = subChart.xAxes.get(0);
			const yAxis = subChart.yAxes.get(0);
			const labelColor = xAxis.labelStyle.color ?? TITLE;
			const lotId = String(subChart.title ?? "");

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
					for (const rs of subChart.renderableSeries.asArray()) {
						// Detach before delete so the shared parent dataSeries survives.
						(rs as { dataSeries?: unknown }).dataSeries = undefined;
					}
					this.parentSurface.removeSubChart(subChart);
					if (lotId) this.restoreLot(lotId);
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
