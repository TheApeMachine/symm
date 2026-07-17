import { useRef } from "react";
import { appStore } from "#/collections/app";
import { resonanceStore } from "#/collections/resonance";
import { heatColor } from "#/components/terminal/canvas";
import { layerCellsFromState } from "#/components/terminal/xray-layers";
import { xrayLayersFromResonance } from "#/components/terminal/xray-view";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

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
				<span data-label class="w-[92px] shrink-0 font-mono text-[10px] text-(--f3)"></span>
				<div data-cells class="grid h-16 flex-1 grid-cols-16 gap-0.5"></div>
				<div class="w-20 shrink-0">
					<div class="flex justify-between font-mono text-[9px] text-(--f4)">
						<span>ε</span>
						<span data-error></span>
					</div>
					<div class="mt-[3px] h-1 overflow-hidden rounded-[2px] bg-(--line)">
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
				cell.className = "min-w-0 rounded-[1px]";
				cells.appendChild(cell);
			}

			cell.style.background = heatColor((value + 1) / 2);
		}
	}
};

/*
XrayHierarchyPanel paints resonance predictive-coding layers from the store
without deriving fake hierarchy from manifold density.
*/
export const XrayHierarchyPanel = () => {
	const symbolRef = useRef<HTMLSpanElement>(null);
	const waitingRef = useRef<HTMLDivElement>(null);
	const rowsRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const symbol = appStore.state.focusSymbol;
			const frame =
				resonanceStore.state.resonance[symbol]?.values().at(-1) ?? null;
			const layers = xrayLayersFromResonance(frame);

			if (symbolRef.current !== null) {
				symbolRef.current.textContent = symbol;
				symbolRef.current.dataset.symbol = symbol;
			}

			if (waitingRef.current !== null && rowsRef.current !== null) {
				paintHierarchyRows(rowsRef.current, waitingRef.current, layers);
			}
		},
		[resonanceStore, appStore],
		[],
	);

	return (
		<div className="shrink-0 px-[18px] py-4">
			<div className="flex items-baseline justify-between gap-3">
				<span className="font-serif font-semibold text-[22px] text-(--f1) leading-[1.1]">
					Predictive-coding hierarchy
				</span>
				<span
					ref={symbolRef}
					className="shrink-0 cursor-pointer font-mono text-[11px] text-(--f3)"
				/>
			</div>
			<div className="mt-1 font-mono text-[10px] text-(--f4)">
				resonance layers only · prediction error ε · not manifold / ρ
			</div>
			<div className="mt-4">
				<div ref={waitingRef} className="font-mono text-[10px] text-(--f4)">
					waiting for resonance layers
				</div>
				<div ref={rowsRef} className="flex flex-col gap-2" />
			</div>
		</div>
	);
};
