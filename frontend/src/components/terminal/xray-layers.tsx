import { heatColor } from "#/components/terminal/canvas";
import { HeatmapRow } from "@/components/ui/heatmap-row";
import type { Variant } from "@/components/ui/types";

const LAYER_NAMES = ["sensory", "micro", "meso", "macro"];

export const semanticLayerName = (index: number, count: number): string => {
	if (index <= 0) {
		return LAYER_NAMES[0] ?? "sensory";
	}

	if (index >= count - 1) {
		return "macro";
	}

	if (count === 3) {
		return "micro";
	}

	return LAYER_NAMES[index] ?? "latent";
};

export const layerCellsFromState = (
	state: unknown,
	cellCount = 16,
): number[] => {
	const values = Array.isArray(state)
		? state.filter((value): value is number => typeof value === "number")
		: [];

	if (values.length === 0 || cellCount <= 0) {
		return [];
	}

	if (values.length === cellCount) {
		return values;
	}

	if (values.length === 1) {
		return Array.from({ length: cellCount }, () => values[0] ?? 0);
	}

	return Array.from({ length: cellCount }, (_, index) => {
		const position = (index / Math.max(cellCount - 1, 1)) * (values.length - 1);
		const left = Math.floor(position);
		const right = Math.min(values.length - 1, left + 1);
		const ratio = position - left;

		return (values[left] ?? 0) * (1 - ratio) + (values[right] ?? 0) * ratio;
	});
};

const layerColor = (value: unknown): string => {
	if (typeof value !== "number") {
		return "var(--line)";
	}

	return heatColor((value + 1) / 2);
};

const layerErrorVariant = (error: unknown): Variant => {
	if (typeof error !== "number") {
		return "info";
	}

	if (error > 0.55) {
		return "error";
	}

	if (error > 0.3) {
		return "warning";
	}

	return "success";
};

const layerErrorValueClass = (variant: Variant): string => {
	if (variant === "error") {
		return "text-(--down)";
	}

	if (variant === "warning") {
		return "text-(--warn)";
	}

	if (variant === "success") {
		return "text-(--up)";
	}

	return "text-(--f4)";
};

export const XrayLayerRows = ({
	layers,
}: {
	layers: Record<string, unknown>[];
}) => (
	<HeatmapRow.Group>
		{layers.map((layer, index) => {
			const state = Array.isArray(layer.state) ? layer.state : [];
			const error = typeof layer.error_norm === "number" ? layer.error_norm : 0;
			const errorVariant = layerErrorVariant(layer.error_norm);
			const layerIndex =
				typeof layer.index === "number" && Number.isFinite(layer.index)
					? layer.index
					: index;
			const label = String(
				layer.name ??
					layer.label ??
					`L${layerIndex} · ${semanticLayerName(index, layers.length)}`,
			);
			const cells = layerCellsFromState(state);
			const errorWidth = Math.min(100, Math.max(0, error * 100));

			return (
				<HeatmapRow
					key={label}
					label={label}
					values={cells}
					colorFn={layerColor}
					metric={
						<HeatmapRow.Metric
							label="ε"
							value={error.toFixed(3)}
							percent={errorWidth}
							variant={errorVariant}
							valueClassName={layerErrorValueClass(errorVariant)}
						/>
					}
				/>
			);
		})}
	</HeatmapRow.Group>
);
