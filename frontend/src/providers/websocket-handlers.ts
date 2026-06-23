import { appStore } from "#/collections/app";
import { cognitiveStore, parseCognitiveFrame } from "#/collections/cognitive";
import { parseWalkTrace, playbookStore } from "#/collections/playbook";
import { parseGaugeFrame, signalStore } from "#/collections/signals";
import { normalizeWireFrame } from "#/components/charts/confidence/gauge-frame";
import { routeWireFrame } from "#/lib/symm/frame-router";

export const finiteCount = (value: unknown): number | null => {
	if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
		return null;
	}

	return Math.floor(value);
};

export type PlaybookBranchWire = {
	branches?: PlaybookBranchWire[];
	[key: string]: unknown;
};

export const isPlaybookBranch = (
	value: unknown,
): value is PlaybookBranchWire => {
	if (typeof value !== "object" || value === null) {
		return false;
	}

	const branch = value as PlaybookBranchWire;

	if (Array.isArray(branch.branches)) {
		return branch.branches.every(isPlaybookBranch);
	}

	return true;
};

export const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

export const gaugeFramesFromState = (
	raw: Record<string, unknown>,
): Record<string, unknown>[] => {
	const gaugeReadings = raw.gauge_readings;

	if (Array.isArray(gaugeReadings)) {
		return gaugeReadings.filter(
			(frame): frame is Record<string, unknown> =>
				typeof frame === "object" && frame !== null,
		);
	}

	const measurements = raw.measurements;

	if (!Array.isArray(measurements)) {
		return [];
	}

	return measurements.filter(
		(frame): frame is Record<string, unknown> =>
			typeof frame === "object" && frame !== null,
	);
};

export const decisionTreeBranches = (
	raw: Record<string, unknown>,
): PlaybookBranchWire[] | null => {
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

const wireString = (frame: Record<string, unknown>, key: string): string => {
	const value = frame[key];

	return typeof value === "string" ? value.trim() : "";
};

const nestedNumber = (
	frame: Record<string, unknown>,
	...path: string[]
): number | null => {
	let current: unknown = frame;

	for (const segment of path) {
		if (!isRecord(current)) {
			return null;
		}

		current = current[segment];
	}

	return typeof current === "number" && Number.isFinite(current)
		? current
		: null;
};

const isGaugeMeasurementFrame = (frame: Record<string, unknown>): boolean => {
	const role = wireString(frame, "role");

	if (role === "measurement") {
		return true;
	}

	const source =
		wireString(frame, "source") ||
		wireString(frame, "origin") ||
		wireString(frame, "Origin");

	if (source === "") {
		return false;
	}

	return nestedNumber(frame, "output", "confidence") !== null;
};

/*
applyGaugeFrame normalizes a measurement or legacy gauge payload and fans it out
to the signal store and per-source SciChart updaters.
*/
export const applyGaugeFrame = (frame: Record<string, unknown>) => {
	const normalized = normalizeWireFrame(frame);
	const source = wireString(normalized, "source");
	const reading = parseGaugeFrame(normalized);

	if (reading !== null) {
		signalStore.actions.updateReading(reading);
	}

	if (source !== "") {
		appStore.actions.stashGaugeFrame(source, normalized);
	}

	appStore.state.confidenceHeatmapUpdater?.(normalized);
	appStore.state.surpriseHeatmapUpdater?.(normalized);
};

const applyCandleFrame = (frame: Record<string, unknown>) => {
	const symbol = wireString(frame, "symbol");
	const updater = appStore.state.candleUpdaters[symbol.trim().toUpperCase()];

	updater?.(frame);
};

const hydrateGaugeFrames = (frame: Record<string, unknown>) => {
	for (const gaugeFrame of gaugeFramesFromState(frame)) {
		applyGaugeFrame(gaugeFrame);
	}
};

const quoteBalanceFromKrakenAssets = (
	rows: unknown[],
	currency: string,
): number | null => {
	const normalizedCurrency = currency.trim().toUpperCase();

	for (const row of rows) {
		if (!isRecord(row)) {
			continue;
		}

		const asset = wireString(row, "asset");
		const balance = nestedNumber(row, "balance");

		if (balance === null) {
			continue;
		}

		if (
			asset === normalizedCurrency ||
			asset === `Z${normalizedCurrency}` ||
			asset === normalizedCurrency.replace("USD", "ZUSD")
		) {
			return balance;
		}
	}

	return null;
};

const walletFrameFromBalances = (
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const normalized: Record<string, unknown> = {
		...frame,
		type: wireString(frame, "type") || "wallet",
	};

	const assets = frame.assets;

	if (isRecord(assets) && Array.isArray(assets.asset)) {
		normalized.assets = assets;

		if (nestedNumber(normalized, "Balance") === null) {
			const currency =
				wireString(normalized, "Currency") ||
				wireString(normalized, "currency") ||
				"USD";
			const balance = quoteBalanceFromKrakenAssets(assets.asset, currency);

			if (balance !== null) {
				normalized.Balance = balance;
			}
		}

		return normalized;
	}

	if (Array.isArray(frame.asset)) {
		const currency =
			wireString(normalized, "Currency") ||
			wireString(normalized, "currency") ||
			"USD";
		const balance = quoteBalanceFromKrakenAssets(frame.asset, currency);

		normalized.assets = {
			asset: frame.asset,
		};

		if (balance !== null) {
			normalized.Balance = balance;
		}

		return normalized;
	}

	return normalized;
};

const applyWalkTrace = (frame: Record<string, unknown>) => {
	const walkTrace = parseWalkTrace(frame);

	if (walkTrace !== null) {
		playbookStore.actions.updateWalkTrace(walkTrace);
	}
};

/*
routeDecodedFrame fans out a decrypted capnp artifact into dashboard stores.
*/
export const routeDecodedFrame = (frame: Record<string, unknown>) => {
	const role = wireString(frame, "role");
	const frameType = wireString(frame, "type");

	if (frameType === "decision_trace" || frameType === "decision_walk") {
		const storyTicks = finiteCount(frame.story_ticks);

		if (storyTicks !== null) {
			appStore.actions.updateStoryTicks(storyTicks);
		}

		const playbookEvaluations = finiteCount(frame.playbook_evaluations);

		if (playbookEvaluations !== null) {
			appStore.actions.updatePlaybookEvaluations(playbookEvaluations);
		}

		routeWireFrame(frame);
		applyWalkTrace(frame);

		return;
	}

	if (isGaugeMeasurementFrame(frame)) {
		applyGaugeFrame(frame);

		return;
	}

	if (role === "story" || frameType === "story_tick") {
		const storyTicks = finiteCount(frame.story_ticks);

		if (storyTicks !== null) {
			appStore.actions.updateStoryTicks(storyTicks);
		}

		const playbookEvaluations = finiteCount(frame.playbook_evaluations);

		if (playbookEvaluations !== null) {
			appStore.actions.updatePlaybookEvaluations(playbookEvaluations);
		}

		return;
	}

	switch (frameType) {
		case "story_tick": {
			const storyTicks = finiteCount(frame.story_ticks);

			if (storyTicks !== null) {
				appStore.actions.updateStoryTicks(storyTicks);
			}

			const playbookEvaluations = finiteCount(frame.playbook_evaluations);

			if (playbookEvaluations !== null) {
				appStore.actions.updatePlaybookEvaluations(playbookEvaluations);
			}

			return;
		}
		case "state":
		case "gauge_confidence":
			hydrateGaugeFrames(frame);

			if (frameType === "state") {
				const playbookEvaluations = finiteCount(frame.playbook_evaluations);

				if (playbookEvaluations !== null) {
					appStore.actions.updatePlaybookEvaluations(playbookEvaluations);
				}
			}

			return;
		case "fluid":
			appStore.state.fluidUpdater?.(frame);

			return;
		case "manifold":
			appStore.actions.stashManifoldFrame(frame);

			return;
		case "regime":
			appStore.actions.stashRegimeFrame(frame);

			return;
		case "resonance_universe":
			appStore.actions.stashResonanceFrame(frame);

			return;
		case "prediction":
			appStore.state.predictionUpdater?.(frame);

			return;
		case "cognitive": {
			const reading = parseCognitiveFrame(frame);

			if (reading !== null) {
				cognitiveStore.actions.updateReading(reading);
			}

			return;
		}
		case "decision_tree": {
			const branches = decisionTreeBranches(frame);

			if (branches !== null) {
				playbookStore.actions.updateBranches(branches);
			}

			return;
		}
		case "ohlc":
			applyCandleFrame(frame);

			return;
		default:
			break;
	}

	if (role === "resonance" || frameType === "resonance_universe") {
		appStore.actions.stashResonanceFrame(frame);

		return;
	}

	if (role === "balances" || role === "wallet") {
		routeWireFrame(walletFrameFromBalances(frame));

		return;
	}

	if (role === "ohlc") {
		applyCandleFrame(frame);

		return;
	}

	routeWireFrame(frame);
};
