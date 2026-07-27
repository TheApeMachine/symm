import { appStore } from "#/collections/app";
import {
	paintTerminalFluidChart,
	repaintTerminalFluidChart,
} from "#/components/charts/fluid";
import { retainManifoldBinary } from "#/providers/manifold-binary";
import { paintHawkes } from "#/components/charts/hawkes";
import { paintTerminalManifoldChart } from "#/components/charts/manifold";
import { paintTerminalPredictionChart } from "#/components/charts/prediction";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import { paintTerminalSignalHeatmap } from "#/components/charts/signal-heatmap";
import { paintSignalDetailMeasurements } from "#/components/kernel/detail";
import {
	paintAllocationBalances,
	paintAllocationCausal,
	paintAllocationPositions,
	paintAllocationInstruments,
	paintAllocationManifold,
	paintAllocationResonance,
} from "#/components/terminal/allocation-surface";
import {
	paintCandidateCausal,
	paintCandidateDecisions,
	paintCandidateManifold,
	paintCandidateResonance,
} from "#/components/terminal/candidate-row";
import { paintCognitiveBeam } from "#/components/terminal/cognitive-beam";
import { paintCortex } from "#/components/terminal/cortex-surface";
import { paintCrossSection } from "#/components/terminal/cross-section-panel";
import {
	paintDashboardPositions,
	paintDashboardLifecycle,
	paintDecisionRows,
} from "#/components/terminal/dashboard-rail";
import {
	paintCausalLadder,
	paintDecisionsEntryLine,
} from "#/components/terminal/decision-side";
import {
	paintDecisions,
	paintDecisionsCausal,
	paintDecisionsInstruments,
	paintDecisionsManifold,
	paintDecisionsMeasurements,
	paintDecisionsResonance,
} from "#/components/terminal/decisions-surface";
import {
	paintHealthMeasurements,
	paintHealthTick,
} from "#/components/terminal/health";
import {
	paintJournalFindings,
	paintJournalPositions,
	paintJournalLifecycle,
} from "#/components/terminal/journal-surface";
import { paintKernelList } from "#/components/terminal/kernel-list";
import {
	paintManifoldMeta,
	repaintManifoldMeta,
	paintResonanceFooter,
	paintResonanceTitle,
} from "#/components/terminal/live-chart-meta";
import {
	paintEngineTick,
	paintOpenCount,
	paintPulseTick,
	paintWalletBalances,
	paintWalletPositions,
	paintWalletTick,
} from "#/components/terminal/live-ticker";
import {
	paintPaletteInstruments,
	paintPaletteMeasurements,
} from "#/components/terminal/palette";
import {
	paintPositions,
	paintPositionStops,
} from "#/components/terminal/position-gauge";
import { paintRegimeRadar } from "#/components/terminal/regime-radar";
import { paintStrategyDecisions } from "#/components/terminal/strategy-decisions";
import { paintThesis } from "#/components/terminal/thesis-modal";
import { paintXrayHawkes } from "#/components/terminal/xray-hawkes";
import { paintXrayHierarchy } from "#/components/terminal/xray-hierarchy";
import { paintXrayLatent } from "#/components/terminal/xray-latent";
import {
	paintXrayFactsCognition,
	paintXrayFactsManifold,
	paintXrayFactsMeasurements,
	paintXrayFactsResonance,
	paintXrayManifold,
	paintXrayManifoldMeasurements,
} from "#/components/terminal/xray-side";
import { FrameHistory } from "#/providers/frame-history";
import { paintManifoldWave } from "#/providers/manifold-parts";
import type { Measurement } from "#/collections/types";

/*
Paint is one imperative target for a backend frame and the current UI focus.
*/
type Paint = (value: unknown | Measurement[], focusSymbol: string) => void;

/*
HistoryPaint marks a target that consumes either full temporal history or one
latest observation per entity instead of only the current websocket delta.
*/
type HistoryPaint = {
	paint: Paint;
	input: "history" | "latest";
};

/*
Drawer owns the primary delta painter and any secondary painters with explicit
input semantics.
*/
type Drawer = {
	paint: Paint | HistoryPaint;
	keys?: Record<string, Paint | HistoryPaint>;
};

/*
viewportHistoryCapacity retains at most one observation per horizontal CSS
pixel for each focused entity, matching the maximum detail a chart can show.
*/
export const viewportHistoryCapacity = (): number => {
	const width = globalThis.innerWidth;

	if (!Number.isFinite(width) || width < 1) {
		throw new Error(`invalid viewport width for frame history: ${width}`);
	}

	return Math.floor(width);
};

/*
drawers map each backend wire key to its paint function(s). The worker forwards
the whole frame; attach walks top-level keys and paints only registered ones.
*/
export const drawers = {
	measurements: {
		paint: paintKernelList,
		keys: {
			hawkes: { paint: paintHawkes, input: "history" },
			signalDetail: {
				paint: paintSignalDetailMeasurements as Paint,
				input: "history",
			},
			regimeRadar: { paint: paintRegimeRadar, input: "history" },
			health: { paint: paintHealthMeasurements, input: "latest" },
			signalHeatmap: {
				paint: paintTerminalSignalHeatmap,
				input: "history",
			},
			xrayHawkes: { paint: paintXrayHawkes, input: "history" },
			xrayFacts: {
				paint: paintXrayFactsMeasurements,
				input: "history",
			},
			xrayManifold: {
				paint: paintXrayManifoldMeasurements,
				input: "latest",
			},
			decisions: {
				paint: paintDecisionsMeasurements,
				input: "latest",
			},
			palette: paintPaletteMeasurements,
			crossSection: {
				paint: paintCrossSection as Paint,
				input: "latest",
			},
		},
	},
	tick: {
		paint: paintPulseTick as Paint,
		keys: {
			open: paintOpenCount as Paint,
			engine: paintEngineTick as Paint,
			wallet: paintWalletTick as Paint,
			health: paintHealthTick,
		},
	},
	balances: {
		paint: paintWalletBalances as Paint,
		keys: {
			allocation: paintAllocationBalances,
		},
	},
	positions: {
		paint: paintWalletPositions as Paint,
		keys: {
			journal: paintJournalPositions,
			dashboard: paintDashboardPositions,
			position: paintPositions,
			allocation: paintAllocationPositions,
		},
	},
	stops: {
		paint: paintPositionStops,
	},
	lifecycle: {
		paint: paintJournalLifecycle,
		keys: {
			dashboard: paintDashboardLifecycle,
		},
	},
	findings: {
		paint: paintJournalFindings,
	},
	decisions: {
		paint: paintStrategyDecisions,
		keys: {
			rows: paintDecisionRows,
			candidate: paintCandidateDecisions,
			surface: paintDecisions,
		},
	},
	causal: {
		paint: { paint: paintCausalLadder, input: "latest" },
		keys: {
			entry: { paint: paintDecisionsEntryLine, input: "latest" },
			candidate: { paint: paintCandidateCausal, input: "latest" },
			surface: { paint: paintDecisionsCausal, input: "latest" },
			allocation: { paint: paintAllocationCausal, input: "latest" },
		},
	},
	resonance: {
		paint: { paint: paintXrayLatent, input: "latest" },
		keys: {
			hierarchy: { paint: paintXrayHierarchy, input: "latest" },
			prediction: { paint: paintTerminalPredictionChart, input: "history" },
			chart: { paint: paintTerminalResonanceChart, input: "latest" },
			footer: { paint: paintResonanceFooter, input: "latest" },
			title: { paint: paintResonanceTitle, input: "latest" },
			facts: { paint: paintXrayFactsResonance, input: "latest" },
			candidate: { paint: paintCandidateResonance, input: "latest" },
			surface: { paint: paintDecisionsResonance, input: "latest" },
			allocation: {
				paint: paintAllocationResonance,
				input: "latest",
			},
		},
	},
	manifold: {
		paint: { paint: paintTerminalFluidChart, input: "latest" },
		keys: {
			chart: { paint: paintTerminalManifoldChart, input: "latest" },
			meta: { paint: paintManifoldMeta, input: "latest" },
			facts: { paint: paintXrayFactsManifold, input: "latest" },
			xray: { paint: paintXrayManifold, input: "latest" },
			candidate: { paint: paintCandidateManifold, input: "latest" },
			surface: { paint: paintDecisionsManifold, input: "latest" },
			allocation: {
				paint: paintAllocationManifold,
				input: "latest",
			},
		},
	},
	// Raw wire packets — not history.project("latest"). These streams have no
	// FrameHistory policy, so "latest" projects to [] and cleared the payloads.
	// Lattice textures arrive as binary SMF1; particles stay off the JSON wire.
	manifold_wave: {
		paint: (value: unknown, focusSymbol: string) => {
			paintManifoldWave(value);
			repaintTerminalFluidChart(focusSymbol);
		},
	},
	manifold_display: {
		paint: (_value: unknown, focusSymbol: string) => {
			repaintTerminalFluidChart(focusSymbol);
		},
	},
	cognition: {
		paint: { paint: paintCortex, input: "latest" },
		keys: {
			beam: { paint: paintCognitiveBeam, input: "latest" },
			facts: { paint: paintXrayFactsCognition, input: "latest" },
		},
	},
	instruments: {
		paint: paintAllocationInstruments,
		keys: {
			surface: paintDecisionsInstruments,
			palette: paintPaletteInstruments,
		},
	},
	diagnostics: {
		paint: paintCrossSection,
	},
} satisfies Record<string, Drawer>;

/*
attach dispatches DRAW frames to drawers, then paintThesis for the thesis shell.
*/
export const attach = (
	worker: Worker,
	history = new FrameHistory(
		viewportHistoryCapacity,
		() => appStore.state.focusSymbol,
	),
) => {
	worker.addEventListener("message", (event: MessageEvent) => {
		const message = event.data as {
			type?: string;
			frame?: Record<string, unknown>;
			buffer?: ArrayBuffer;
		};
		const focusSymbol = appStore.state.focusSymbol;

		if (message.type === "DRAW_BIN" && message.buffer instanceof ArrayBuffer) {
			if (retainManifoldBinary(message.buffer) === null) {
				return;
			}

			repaintTerminalFluidChart(focusSymbol);
			repaintManifoldMeta(focusSymbol);
			return;
		}

		if (message.type !== "DRAW" || message.frame === undefined) {
			return;
		}

		for (const [name, value] of Object.entries(message.frame)) {
			const drawer = drawers[name as keyof typeof drawers] as
				| Drawer
				| undefined;

			if (drawer === undefined) {
				continue;
			}

			history.retain(name, value);
			const projections = new Map<"history" | "latest", unknown>();
			const targets = [drawer.paint, ...Object.values(drawer.keys ?? {})];

			for (const target of targets) {
				if (typeof target === "function") {
					target(value, focusSymbol);
					continue;
				}

				const projection =
					projections.get(target.input) ?? history.project(name, target.input);
				projections.set(target.input, projection);
				target.paint(projection, focusSymbol);
			}
		}

		paintThesis(message.frame, focusSymbol);
	});
};
