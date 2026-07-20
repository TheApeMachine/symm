import { appStore } from "#/collections/app";
import { paintTerminalFluidChart } from "#/components/charts/fluid";
import { paintHawkes } from "#/components/charts/hawkes";
import { paintTerminalManifoldChart } from "#/components/charts/manifold";
import { paintTerminalPredictionChart } from "#/components/charts/prediction";
import { paintTerminalResonanceChart } from "#/components/charts/resonance";
import { paintTerminalSignalHeatmap } from "#/components/charts/signal-heatmap";
import { paintSignalDetailMeasurements } from "#/components/kernel/detail";
import { paintCandidateCausal, paintCandidateDecisions, paintCandidateManifold, paintCandidateResonance } from "#/components/terminal/candidate-row";
import { paintCognitiveBeam } from "#/components/terminal/cognitive-beam";
import { paintCrossSection } from "#/components/terminal/cross-section-panel";
import {
	paintDashboardHoldings,
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
	paintJournalHoldings,
	paintJournalLifecycle,
} from "#/components/terminal/journal-surface";
import { paintKernelList } from "#/components/terminal/kernel-list";
import {
	paintManifoldMeta,
	paintResonanceFooter,
	paintResonanceTitle,
} from "#/components/terminal/live-chart-meta";
import {
	paintEngineTick,
	paintOpenCount,
	paintPulseTick,
	paintWalletBalances,
	paintWalletHoldings,
	paintWalletTick,
} from "#/components/terminal/live-ticker";
import {
	paintPaletteInstruments,
	paintPaletteMeasurements,
} from "#/components/terminal/palette";
import {
	paintPositionHoldings,
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
import {
	paintAllocationBalances,
	paintAllocationCausal,
	paintAllocationHoldings,
	paintAllocationInstruments,
	paintAllocationManifold,
	paintAllocationResonance,
} from "#/routes/allocation";
import { paintCortex } from "#/routes/cortex";

type Paint = (value: unknown, focusSymbol: string) => void;

type Drawer = {
	paint: Paint;
	keys?: Record<string, Paint>;
};

/*
drawers map each backend wire key to its paint function(s). The worker forwards
the whole frame; attach walks top-level keys and paints only registered ones.
*/
export const drawers = {
	measurements: {
		paint: paintKernelList,
		keys: {
			hawkes: paintHawkes,
			signalDetail: paintSignalDetailMeasurements,
			regimeRadar: paintRegimeRadar,
			health: paintHealthMeasurements,
			signalHeatmap: paintTerminalSignalHeatmap,
			xrayHawkes: paintXrayHawkes,
			xrayFacts: paintXrayFactsMeasurements,
			xrayManifold: paintXrayManifoldMeasurements,
			decisions: paintDecisionsMeasurements,
			palette: paintPaletteMeasurements,
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
	holdings: {
		paint: paintWalletHoldings as Paint,
		keys: {
			journal: paintJournalHoldings,
			dashboard: paintDashboardHoldings,
			position: paintPositionHoldings,
			allocation: paintAllocationHoldings,
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
		paint: paintCausalLadder,
		keys: {
			entry: paintDecisionsEntryLine,
			candidate: paintCandidateCausal,
			surface: paintDecisionsCausal,
			allocation: paintAllocationCausal,
		},
	},
	resonance: {
		paint: paintXrayLatent,
		keys: {
			hierarchy: paintXrayHierarchy,
			prediction: paintTerminalPredictionChart,
			chart: paintTerminalResonanceChart,
			footer: paintResonanceFooter,
			title: paintResonanceTitle,
			facts: paintXrayFactsResonance,
			candidate: paintCandidateResonance,
			surface: paintDecisionsResonance,
			allocation: paintAllocationResonance,
		},
	},
	manifold: {
		paint: paintTerminalFluidChart,
		keys: {
			chart: paintTerminalManifoldChart,
			meta: paintManifoldMeta,
			facts: paintXrayFactsManifold,
			xray: paintXrayManifold,
			candidate: paintCandidateManifold,
			surface: paintDecisionsManifold,
			allocation: paintAllocationManifold,
		},
	},
	cognition: {
		paint: paintCortex,
		keys: {
			beam: paintCognitiveBeam,
			facts: paintXrayFactsCognition,
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
export const attach = (worker: Worker) => {
	worker.addEventListener("message", (event: MessageEvent) => {
		const message = event.data as {
			type?: string;
			frame?: Record<string, unknown>;
		};

		if (message.type !== "DRAW" || message.frame === undefined) {
			return;
		}

		const focusSymbol = appStore.state.focusSymbol;

		for (const [name, value] of Object.entries(message.frame)) {
			const drawer = drawers[name as keyof typeof drawers] as
				| Drawer
				| undefined;

			if (drawer === undefined) {
				continue;
			}

			drawer.paint(value, focusSymbol);

			if (drawer.keys === undefined) {
				continue;
			}

			for (const paint of Object.values(drawer.keys)) {
				paint(value, focusSymbol);
			}
		}

		paintThesis(message.frame, focusSymbol);
	});
};
