const layerColor = (value: unknown): string => {
	if (typeof value !== "number") {
		return "var(--line)";
	}

	return `color-mix(in srgb, var(--acc) ${Math.round((value + 1) * 50)}%, var(--sunken))`;
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
	<div className="flex flex-col gap-2.5">
		{layers.map((layer, index) => {
			const state = Array.isArray(layer.state) ? layer.state : [];
			const error = typeof layer.error_norm === "number" ? layer.error_norm : 0;
			const errorTone = layerErrorTone(layer.error_norm);
			const label = String(
				layer.name ?? layer.label ?? `L${layer.index ?? index}`,
			);

			return (
				<div key={label} className="flex items-center gap-3">
					<span className="w-[92px] shrink-0 font-mono text-[10px] text-(--f3)">
						{label}
					</span>
					<div className="grid flex-1 grid-cols-16 gap-0.5">
						{state.map((value, bin) => (
							<div
								key={`${bin}-${label}`}
								className="aspect-square rounded-[1px]"
								style={{ background: layerColor(value) }}
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
									width: `${error * 100}%`,
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
