import type { TerminalResonanceLayer } from "#/components/terminal/chart-data";
import { terminalColormap } from "#/components/terminal/tmp-draw";

const LAYER_LABELS = ["sensory", "micro", "meso", "macro"];
const LAYER_BINS = Array.from({ length: 16 }, (_, bin) => bin);

const layerColor = (value: number): string => {
	const rgb = terminalColormap((value + 1) / 2);
	return `rgb(${rgb[0]},${rgb[1]},${rgb[2]})`;
};

const layerErrorTone = (error: number): string => {
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
	layers: TerminalResonanceLayer[];
}) => (
	<div className="flex flex-col gap-2.5">
		{LAYER_LABELS.map((label, index) => {
			const layer = layers[index];

			if (!layer) {
				return null;
			}

			const errorPercent = Math.round(layer.errorNorm * 100);
			const errorTone = layerErrorTone(layer.errorNorm);

			return (
				<div key={label} className="flex items-center gap-3">
					<span className="w-[92px] shrink-0 font-mono text-[10px] text-(--f3)">
						L{index} · {label}
					</span>
					<div className="grid flex-1 grid-cols-16 gap-0.5">
						{LAYER_BINS.map((bin) => {
							const value = layer.state[bin] ?? 0;

							return (
								<div
									key={`${label}-${bin}`}
									className="aspect-square rounded-[1px]"
									style={{ background: layerColor(value) }}
								/>
							);
						})}
					</div>
					<div className="w-20 shrink-0">
						<div className="flex justify-between font-mono text-[9px] text-(--f4)">
							<span>ε</span>
							<span style={{ color: errorTone }}>
								{layer.errorNorm.toFixed(3)}
							</span>
						</div>
						<div className="mt-[3px] h-1 overflow-hidden rounded-[2px] bg-(--line)">
							<div
								className="h-full"
								style={{
									width: `${errorPercent}%`,
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
