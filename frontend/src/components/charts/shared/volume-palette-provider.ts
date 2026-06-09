import {
	EBaseType,
	EFillPaletteMode,
	EPaletteProviderType,
	type IFillPaletteProvider,
	type IPointMetadata,
	type IRenderableSeries,
	type OhlcDataSeries,
	parseColorToUIntArgb,
	registerType,
	type TPaletteProviderDefinition,
} from "scichart";

const VOLUME_PALETTE_PROVIDER_TYPE = "TradingAnnotationVolumePaletteProvider";
const VIVID_GREEN = "#67BDAF";
const VIVID_RED = "#C52E60";

export class VolumePaletteProvider implements IFillPaletteProvider {
	public readonly fillPaletteMode: EFillPaletteMode = EFillPaletteMode.SOLID;
	private readonly ohlc: OhlcDataSeries;
	private readonly upColorArgb: number;
	private readonly downColorArgb: number;

	constructor(ohlc: OhlcDataSeries, upColor: string, downColor: string) {
		this.ohlc = ohlc;
		this.upColorArgb = parseColorToUIntArgb(upColor);
		this.downColorArgb = parseColorToUIntArgb(downColor);
	}

	public onAttached(_parentSeries: IRenderableSeries): void {}

	public onDetached(): void {}

	public overrideFillArgb(
		_xValue: number,
		_yValue: number,
		index: number,
		_opacity?: number,
		_metadata?: IPointMetadata,
	): number {
		const nativeOpen = this.ohlc.getNativeOpenValues();
		const nativeClose = this.ohlc.getNativeCloseValues();
		const open = nativeOpen.get(index);
		const close = nativeClose.get(index);

		if (close >= open) {
			return this.upColorArgb;
		}

		return this.downColorArgb;
	}

	public overrideStrokeArgb(
		xValue: number,
		yValue: number,
		index: number,
		opacity?: number,
		metadata?: IPointMetadata,
	): number {
		return this.overrideFillArgb(xValue, yValue, index, opacity, metadata);
	}

	public toJSON(): TPaletteProviderDefinition {
		const barCount = this.ohlc.count();
		const nativeOpen = this.ohlc.getNativeOpenValues();
		const nativeClose = this.ohlc.getNativeCloseValues();
		const isUpByIndex = Array.from({ length: barCount }, (_, pointIndex) => {
			const open = nativeOpen.get(pointIndex);
			const close = nativeClose.get(pointIndex);

			return close >= open;
		});

		return {
			type: EPaletteProviderType.Custom,
			customType: VOLUME_PALETTE_PROVIDER_TYPE,
			options: {
				isUpByIndex,
				upColor: `${VIVID_GREEN}66`,
				downColor: `${VIVID_RED}66`,
			},
		};
	}
}

class DeserializedVolumePaletteProvider implements IFillPaletteProvider {
	public readonly fillPaletteMode: EFillPaletteMode = EFillPaletteMode.SOLID;
	private readonly isUpByIndex: boolean[];
	private readonly upColorArgb: number;
	private readonly downColorArgb: number;

	constructor(isUpByIndex: boolean[], upColor: string, downColor: string) {
		this.isUpByIndex = isUpByIndex;
		this.upColorArgb = parseColorToUIntArgb(upColor);
		this.downColorArgb = parseColorToUIntArgb(downColor);
	}

	public onAttached(_parentSeries: IRenderableSeries): void {}

	public onDetached(): void {}

	public overrideFillArgb(
		_xValue: number,
		_yValue: number,
		index: number,
		_opacity?: number,
		_metadata?: IPointMetadata,
	): number {
		if (this.isUpByIndex[index]) {
			return this.upColorArgb;
		}

		return this.downColorArgb;
	}

	public overrideStrokeArgb(
		xValue: number,
		yValue: number,
		index: number,
		opacity?: number,
		metadata?: IPointMetadata,
	): number {
		return this.overrideFillArgb(xValue, yValue, index, opacity, metadata);
	}

	public toJSON(): TPaletteProviderDefinition {
		return {
			type: EPaletteProviderType.Custom,
			customType: VOLUME_PALETTE_PROVIDER_TYPE,
			options: {
				isUpByIndex: this.isUpByIndex,
				upColor: `${VIVID_GREEN}66`,
				downColor: `${VIVID_RED}66`,
			},
		};
	}
}

registerType(
	EBaseType.PaletteProvider,
	VOLUME_PALETTE_PROVIDER_TYPE,
	(options?: { isUpByIndex?: boolean[] }) => {
		const isUpByIndex = options?.isUpByIndex ?? [];

		return new DeserializedVolumePaletteProvider(
			isUpByIndex,
			`${VIVID_GREEN}66`,
			`${VIVID_RED}66`,
		);
	},
	true,
);
