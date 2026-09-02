import { createRef } from "react";
import type { ResonanceFrame } from "#/collections/types";
import { heatColor } from "#/components/terminal/canvas";
import { layerCellsFromState } from "#/components/terminal/xray-layers";
import { xrayLayersFromResonance } from "#/components/terminal/xray-view";
import { Typography } from "@/components/ui/typography";

const symbolRef = createRef<HTMLSpanElement>();
const waitingRef = createRef<HTMLDivElement>();
const rowsRef = createRef<HTMLDivElement>();

/*
layerErrorTone maps prediction-error magnitude onto terminal semantic colors.
*/
const layerErrorTone = (error: number): string => {
	if (error > 0.55) {
		return "var(--down)";
	}

	if (error > 0.3) {
		return "var(--warn)";
	}

	return "var(--up)";
};

/*
paintHierarchyRows syncs resonance layer rows into the host without React diffs.
*/
const paintHierarchyRows = (
	host: HTMLDivElement,
	waiting: HTMLDivElement,
	layers: ReturnType<typeof xrayLayersFromResonance>,
): void => {
	waiting.style.display = layers.length === 0 ? "" : "none";
	host.style.display = layers.length === 0 ? "none" : "";

	while (host.childElementCount > layers.length) {
		host.lastElementChild?.remove();
	}

	for (const [index, layer] of layers.entries()) {
		let row = host.children[index] as HTMLDivElement | undefined;

		if (row === undefined) {
			row = document.createElement("div");
			row.className = "flex items-center gap-3";
			row.innerHTML = `
				<span data-label class="w-23 shrink-0 font-mono text-[10px] text-(--f3)"></span>
				<div data-cells class="grid flex-1 grid-cols-16 gap-0.5"></div>
				<div class="w-20 shrink-0">
					<div class="flex justify-between font-mono text-[9px] text-(--f4)">
						<span>ε</span>
						<span data-error></span>
					</div>
					<div class="mt-0.75 h-1 overflow-hidden rounded-xs bg-(--line)">
						<div data-fill class="h-full" style="width:0%"></div>
					</div>
				</div>
			`;
			host.appendChild(row);
		}

		const label = row.querySelector<HTMLSpanElement>("[data-label]");
		const cells = row.querySelector<HTMLDivElement>("[data-cells]");
		const error = row.querySelector<HTMLSpanElement>("[data-error]");
		const fill = row.querySelector<HTMLDivElement>("[data-fill]");
		const values = layerCellsFromState(layer.state);

		if (label !== null) {
			label.textContent = layer.label;
		}

		if (error !== null) {
			error.textContent = layer.error_norm.toFixed(3);
			error.style.color = layerErrorTone(layer.error_norm);
		}

		if (fill !== null) {
			fill.style.width = `${Math.min(100, Math.max(0, layer.error_norm * 100))}%`;
			fill.style.background = layerErrorTone(layer.error_norm);
		}

		if (cells === null) {
			continue;
		}

		while (cells.childElementCount > values.length) {
			cells.lastElementChild?.remove();
		}

		for (const [cellIndex, value] of values.entries()) {
			let cell = cells.children[cellIndex] as HTMLDivElement | undefined;

			if (cell === undefined) {
				cell = document.createElement("div");
				cell.className =
					"min-w-0 aspect-square rounded-[1px] transition-colors duration-150 ease-out";
				cells.appendChild(cell);
			}

			cell.style.background = heatColor((value + 1) / 2);
			cell.style.boxShadow = "";
		}
	}
};

/*
paintXrayHierarchy paints the focused symbol's resonance layers into the
hierarchy host.

The resonance store supplies one retained carrier per symbol. This painter reads
that complete bounded snapshot and owns no second cache of backend state.
*/
export const paintXrayHierarchy = (value: unknown, focusSymbol: string) => {
	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Array<ResonanceFrame | Record<string, unknown>>;

	const focused = frames.find((frame) => frame.symbol === focusSymbol);
	const layers = xrayLayersFromResonance(focused);

	if (symbolRef.current !== null) {
		symbolRef.current.textContent = focusSymbol;
		symbolRef.current.dataset.symbol = focusSymbol;
	}

	if (waitingRef.current !== null && rowsRef.current !== null) {
		waitingRef.current.textContent =
			frames.length === 0
				? "waiting for resonance layers"
				: `no resonance carrier for ${focusSymbol}`;
		paintHierarchyRows(rowsRef.current, waitingRef.current, layers);
	}
};

/*
XrayHierarchyPanel is the static predictive-coding hierarchy shell. DRAW paints
via paintXrayHierarchy.
*/
export const XrayHierarchyPanel = () => (
	<div className="shrink-0 px-4.5 py-4">
		<div className="flex items-baseline justify-between gap-3">
			<Typography.Display size="lg">
				Predictive-coding hierarchy
			</Typography.Display>
			<span
				ref={symbolRef}
				className="shrink-0 cursor-pointer font-mono text-[11px] text-(--f3)"
			/>
		</div>
		<div className="mt-1 font-mono text-[10px] text-(--f4)">
			latent state · prediction error ε per layer · macro = abstract regime,
			sensory = raw tape
		</div>
		<div className="mt-4">
			<div ref={waitingRef} className="font-mono text-[10px] text-(--f4)">
				waiting for resonance layers
			</div>
			<div ref={rowsRef} className="flex flex-col gap-2" />
		</div>
	</div>
);
