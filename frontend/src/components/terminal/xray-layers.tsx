import { heatColor } from "#/components/terminal/canvas";

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

const layerErrorTone = (error: unknown): string => {
	if (typeof error !== "number") {
		return "var(--f4)";
	}

	if (error > 0.55) {
		return "var(--down)";
	}

	if (error > 0.3) {
		return "var(--warn)";
	}

	return "var(--up)";
};

export const XrayLayerRows = ({
	layers,
}: {
	layers: Record<string, unknown>[];
}) => (
	<div className="flex flex-col gap-2">
		{layers.map((layer, index) => {
			const state = Array.isArray(layer.state) ? layer.state : [];
			const error = typeof layer.error_norm === "number" ? layer.error_norm : 0;
			const errorTone = layerErrorTone(layer.error_norm);
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
			const occurrences = new Map<string, number>();
			const keyedCells = cells.map((value) => {
				const valueKey = value.toFixed(6);
				const count = occurrences.get(valueKey) ?? 0;

				occurrences.set(valueKey, count + 1);

				return {
					key: `${label}-${valueKey}-${count}`,
					value,
				};
			});
			const errorWidth = Math.min(100, Math.max(0, error * 100));

			return (
				<div key={label} className="flex items-center gap-3">
					<span className="w-[92px] shrink-0 font-mono text-[10px] text-(--f3)">
						{label}
					</span>
					<div className="grid h-16 flex-1 grid-cols-16 gap-0.5">
						{keyedCells.map((cell) => (
							<div
								key={cell.key}
								className="min-w-0 rounded-[1px]"
								style={{ background: layerColor(cell.value) }}
							/>
						))}
					</div>
					<div className="w-20 shrink-0">
						<div className="flex justify-between font-mono text-[9px] text-(--f4)">
							<span>ε</span>
							<span style={{ color: errorTone }}>{error.toFixed(3)}</span>
						</div>
						<div className="mt-[3px] h-1 overflow-hidden rounded-[2px] bg-(--line)">
							<div
								className="h-full"
								style={{
									width: `${errorWidth}%`,
									background: errorTone,
								}}
							/>
						</div>
					</div>
				</div>
			);
		})}
	</div>
);
