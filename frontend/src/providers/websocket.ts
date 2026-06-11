import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { appStore } from "#/collections/app";
import { balanceStore } from "#/collections/balance";
import {
	type PlaybookBranch,
	parseWalkTrace,
	playbookStore,
} from "#/collections/playbook";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const finiteCount = (value: unknown): number | null => {
	if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
		return null;
	}

	return Math.floor(value);
};

const isPlaybookBranch = (value: unknown): value is PlaybookBranch => {
	if (typeof value !== "object" || value === null) {
		return false;
	}

	const branch = value as PlaybookBranch;

	if (Array.isArray(branch.branches)) {
		return branch.branches.every(isPlaybookBranch);
	}

	return true;
};

const decisionTreeBranches = (
	raw: Record<string, unknown>,
): PlaybookBranch[] | null => {
	const topLevel = raw.branches;

	if (Array.isArray(topLevel) && topLevel.every(isPlaybookBranch)) {
		return topLevel;
	}

	const nested = raw.value;

	if (typeof nested === "object" && nested !== null) {
		const nestedBranches = (nested as Record<string, unknown>).branches;

		if (
			Array.isArray(nestedBranches) &&
			nestedBranches.every(isPlaybookBranch)
		) {
			return nestedBranches;
		}
	}

	return null;
};

export const WsFeed = () => {
	const { updateOnline, updatePlaybookEvaluations, updateStoryTicks } =
		appStore.actions;
	const { updateBranches, updateWalkTrace } = playbookStore.actions;

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => updateOnline(true),
		onClose: () => updateOnline(false),
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				switch (raw.type) {
					case "balances":
						balanceStore.setState((prev) => ({ ...prev, ...raw }));
						break;
					case "story": {
						const storyTicks = finiteCount(raw.story_ticks);

						if (storyTicks !== null) {
							updateStoryTicks(storyTicks);
						}

						const playbookEvaluations = finiteCount(raw.playbook_evaluations);

						if (playbookEvaluations !== null) {
							updatePlaybookEvaluations(playbookEvaluations);
						}

						break;
					}
					case "decision_tree": {
						const branches = decisionTreeBranches(raw);

						if (branches !== null) {
							updateBranches(branches);
						}

						break;
					}
					case "decision_walk": {
						const walkTrace = parseWalkTrace(raw);

						if (walkTrace !== null) {
							updateWalkTrace(walkTrace);
						}

						break;
					}
					case "ohlc":
						appStore.state.candleUpdater?.(raw);
						break;
					case "gauge": {
						const source = typeof raw.source === "string" ? raw.source : "";

						if (source !== "") {
							appStore.state.gaugeUpdaters[source]?.(raw);
						}

						appStore.state.confidenceHeatmapUpdater?.(raw);
						appStore.state.surpriseHeatmapUpdater?.(raw);
						break;
					}
					case "regime":
						appStore.state.regimeUpdater?.(raw);
						break;
					case "fluid":
						appStore.state.fluidUpdater?.(raw);
						break;
					case "manifold":
						appStore.state.manifoldUpdater?.(raw);
						break;
					case "prediction":
						appStore.state.predictionUpdater?.(raw);
						break;
					default:
						break;
				}
			} catch (error) {
				console.error("websocket frame parse failed", error, event.data);
			}
		},
	});

	return null;
};
