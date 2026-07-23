import { createStore } from "@tanstack/react-store";
import { DEFAULT_FOCUS_SYMBOL } from "#/collections/app";

export type TerminalSurface =
	| "dashboard"
	| "signals"
	| "decisions"
	| "journal"
	| "xray"
	| "cortex"
	| "allocation";

export { DEFAULT_FOCUS_SYMBOL };

/*
FluidFieldLayer names the physically distinct pilot-wave projections available
for inspection; Composite preserves both by drawing rather than adding them.
*/
export type FluidFieldLayer = "Composite" | "Coherence" | "Gas";

const FIELD_LAYERS: FluidFieldLayer[] = ["Composite", "Coherence", "Gas"];

export const terminalStore = createStore(
	{
		scanlines: true,
		fieldStyle: "Heatmap" as "Heatmap" | "Contour",
		fieldLayer: "Composite" as FluidFieldLayer,
		selectedSource: "manifold",
		inspectorSource: null as string | null,
		paletteOpen: false,
		paletteMode: "all" as "all" | "symbols",
		paletteQuery: "",
		paletteIndex: 0,
		focusSymbol: DEFAULT_FOCUS_SYMBOL,
		thesisSymbol: null as string | null,
	},
	({ setState }) => ({
		toggleScanlines: () =>
			setState((prev) => ({
				...prev,
				scanlines: !prev.scanlines,
			})),
		toggleFieldStyle: () =>
			setState((prev) => ({
				...prev,
				fieldStyle: prev.fieldStyle === "Heatmap" ? "Contour" : "Heatmap",
			})),
		cycleFieldLayer: () =>
			setState((prev) => ({
				...prev,
				fieldLayer:
					FIELD_LAYERS[
						(FIELD_LAYERS.indexOf(prev.fieldLayer) + 1) % FIELD_LAYERS.length
					],
			})),
		selectSource: (selectedSource: string) =>
			setState((prev) => ({
				...prev,
				selectedSource,
			})),
		inspectSource: (source: string) =>
			setState((prev) => ({
				...prev,
				selectedSource: source,
				inspectorSource: source,
			})),
		closeInspect: () =>
			setState((prev) => ({
				...prev,
				inspectorSource: null,
			})),
		openPalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "all",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		openSymbolPalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "symbols",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		closePalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: false,
			})),
		setPaletteQuery: (paletteQuery: string) =>
			setState((prev) => ({
				...prev,
				paletteQuery,
				paletteIndex: 0,
			})),
		bumpPaletteIndex: (delta: number) =>
			setState((prev) => ({
				...prev,
				paletteIndex: prev.paletteIndex + delta,
			})),
		selectFocusSymbol: (focusSymbol: string) =>
			setState((prev) => ({
				...prev,
				focusSymbol,
			})),
		openThesis: (thesisSymbol: string) =>
			setState((prev) => ({
				...prev,
				thesisSymbol,
			})),
		closeThesis: () =>
			setState((prev) => ({
				...prev,
				thesisSymbol: null,
			})),
	}),
);
