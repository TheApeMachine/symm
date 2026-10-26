import { signals } from "#/collections/app";
import {
	ingestCognitionFrame,
	ingestPathWalks,
	ingestS3Prefix,
	ingestTrainRegions,
} from "#/collections/cognition";
import {
	beginLongtailHistory,
	endLongtailHistory,
	ingestLongtailHistory,
	ingestLongtailTick,
} from "#/collections/longtail";
import { ingestPaperRecord, parsePaperRecord } from "#/collections/paper";
import {
	ingestSequiturFrame,
	ingestTrainingStep,
} from "#/collections/sequitur";
import { tryDecodeJsonBytes } from "#/lib/json-bytes";
import { ingestDeskSnapshot } from "#/lib/lifecycle";
import { syncOpenPositionCount } from "#/lib/open-position-count";
import type { Value } from "#/types/value";

const watchScopes = (
	store: (typeof signals)[string],
	project: (value: Value) => void,
) => {
	let prior: Record<string, Value> = {};
	store.subscribe((state) => {
		for (const [scope, frame] of Object.entries(state)) {
			if (!frame || frame === prior[scope]) continue;
			project(frame);
		}
		prior = state;
	});
};

const decodeLayers = (value: Value, onLayer: (payload: unknown) => void) => {
	for (const { payload } of value.readings()) {
		onLayer(payload);
	}
};

const projectTrainRegions = (value: Value) => {
	if (!value.index.some((index) => index.includes("train/regions"))) {
		return;
	}
	for (let i = 0; i < value.index.length; i++) {
		if (!value.index[i].includes("train/regions")) continue;
		const payload = tryDecodeJsonBytes(value.layers[i]);
		if (payload === undefined) continue;
		ingestTrainRegions(Number(value.tick), value.index[i], payload);
	}
};

watchScopes(signals.paper, (value) => {
	const record = parsePaperRecord(
		value.role,
		value.scope,
		value.index.map((index, layerIndex) => ({
			index,
			bytes: value.layers[layerIndex],
		})),
	);
	ingestPaperRecord(record);
});

watchScopes(signals.sequitur, (value) => {
	decodeLayers(value, ingestSequiturFrame);
});

watchScopes(signals.cognition, (value) => {
	decodeLayers(value, (payload) => {
		if (
			value.role === "match" &&
			payload &&
			typeof payload === "object" &&
			!Array.isArray(payload)
		) {
			ingestCognitionFrame(value.role, {
				...(payload as Record<string, unknown>),
				epoch: String(value.epoch),
			});
			return;
		}
		ingestCognitionFrame(value.role, payload);
	});
});

watchScopes(signals.s3, (value) => {
	ingestS3Prefix(value);
});

watchScopes(signals.training, (value) => {
	if (value.role === "longtail") {
		if (value.scope === "history-begin") {
			beginLongtailHistory();
			return;
		}
		if (value.scope === "history-end") {
			endLongtailHistory();
			return;
		}
		decodeLayers(value, (payload) => {
			if (value.scope === "history") {
				ingestLongtailHistory(payload);
				return;
			}
			ingestLongtailTick(payload);
		});
		return;
	}
	if (value.role === "walk") {
		decodeLayers(value, (payload) =>
			ingestPathWalks(Number(value.tick), payload),
		);
		return;
	}
	if (value.role === "step") {
		ingestTrainingStep(value);
	}
	projectTrainRegions(value);
});

watchScopes(signals.workspace, projectTrainRegions);

let positionsFlush: ReturnType<typeof setTimeout> | null = null;

const flushPositions = () => {
	positionsFlush = null;
	syncOpenPositionCount();
	ingestDeskSnapshot();
};

watchScopes(signals.positions, () => {
	if (positionsFlush !== null) {
		return;
	}

	positionsFlush = setTimeout(flushPositions, 50);
});
