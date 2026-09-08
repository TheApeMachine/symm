import { createStore } from "@tanstack/react-store";
import { ByteBuffer } from "flatbuffers";
import {
	LearningState,
	type LearningStateT,
} from "#/providers/telemetry/telemetry/learning-state";

// The decoded FlatBuffer is the single learning state consumed by every surface.
export const learningStore = createStore<LearningStateT | null>(null);

export const receiveLearning = (bytes: Uint8Array) => {
	const state = LearningState.getRootAsLearningState(
		new ByteBuffer(bytes),
	).unpack();

	learningStore.setState(() => state);
};
