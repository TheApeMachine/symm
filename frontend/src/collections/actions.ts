import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

export type Action = {
	id: string;
	tick: number;
	symbol: string;
	type: string;
	side: string;
	verdict: string;
	reason: string;
	score: number;
	entryLine: number;
	entryScore: number;
	entryConfidence: number;
	fraction: number;
	price: number;
	branchKey: string;
	reasonSource: string;
	reasonCategory: string;
	decisionAt: string;
};

type ActionFrame = Record<string, unknown>;

const asFrame = (value: unknown): ActionFrame | null =>
	typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as ActionFrame)
		: null;

const frameValue = (frame: ActionFrame, ...keys: string[]): unknown => {
	for (const key of keys) {
		if (frame[key] !== undefined) {
			return frame[key];
		}
	}

	return undefined;
};

const stringValue = (value: unknown): string | null =>
	typeof value === "string" && value.trim() !== "" ? value : null;

const finite = (value: unknown): number | null => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : null;
};

const parseAction = (value: unknown): Action | null => {
	const frame = asFrame(value);

	if (frame === null) {
		return null;
	}

	const symbol = stringValue(frameValue(frame, "symbol", "Symbol"));
	const side = stringValue(frameValue(frame, "side", "Action", "action"));

	if (symbol === null || side === null) {
		return null;
	}

	const tick = finite(frameValue(frame, "tick", "Tick")) ?? 0;
	const score = finite(frameValue(frame, "score", "Edge", "edge")) ?? 0;
	const entryScore =
		finite(frameValue(frame, "entryScore", "entry_score", "Edge", "edge")) ??
		score;
	const entryConfidence =
		finite(
			frameValue(
				frame,
				"entryConfidence",
				"entry_confidence",
				"Confidence",
				"confidence",
			),
		) ?? 0;
	const fraction = finite(frameValue(frame, "fraction", "Size", "size")) ?? 0;
	const verdict =
		stringValue(frameValue(frame, "verdict", "Verdict")) ??
		(side === "hold" ? "blocked" : "allow");
	const id =
		stringValue(frameValue(frame, "id", "ID")) ??
		`${tick}:${symbol}:${side}:${score}:${entryScore}:${entryConfidence}:${fraction}`;

	return {
		id,
		tick,
		symbol,
		type: stringValue(frameValue(frame, "type", "Type")) ?? "intent",
		side,
		verdict,
		reason:
			stringValue(frameValue(frame, "reason", "Reason")) ?? "planner_intent",
		score,
		entryLine: finite(frameValue(frame, "entryLine", "entry_line")) ?? 0,
		entryScore,
		entryConfidence,
		fraction,
		price: finite(frameValue(frame, "price", "Price")) ?? 0,
		branchKey:
			stringValue(frameValue(frame, "branchKey", "branch_key")) ??
			"planner/intent",
		reasonSource:
			stringValue(frameValue(frame, "reasonSource", "reason_source")) ??
			"planner",
		reasonCategory:
			stringValue(frameValue(frame, "reasonCategory", "reason_category")) ??
			side,
		decisionAt:
			stringValue(frameValue(frame, "decisionAt", "decision_at")) ?? "",
	};
};

export const normalizeActions = (frame: unknown): Action[] => {
	const frames = Array.isArray(frame) ? frame : [frame];
	const actions = frames.flatMap((action) => {
		const parsed = parseAction(action);

		return parsed === null ? [] : [parsed];
	});

	if (actions.length !== frames.length) {
		console.warn("actionStore: skipped malformed action rows", {
			received: frames.length,
			accepted: actions.length,
		});
	}

	return actions;
};

/*
actionStore is the single source of truth for frontend trading decisions.
Backend action and planner intent frames are routed here and indexed by symbol.
*/
export const actionStore = createStore(
	{
		actions: {} as Record<string, ReturnType<typeof Circular<Action>>>,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const frames = normalizeActions(frame);
				const actions = { ...prev.actions };

				for (const frame of frames) {
					if (!actions[frame.symbol]) {
						actions[frame.symbol] = Circular<Action>(50);
					}

					if (
						actions[frame.symbol]
							.values()
							.some((action) => action.id === frame.id)
					) {
						continue;
					}

					actions[frame.symbol].push(frame);
				}

				return {
					actions,
				};
			}),
		reset: () =>
			setState(() => ({
				actions: {},
			})),
	}),
);
