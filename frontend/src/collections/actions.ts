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

/*
actionStore is the single source of truth for all signal data.
Every Action from the backend WebSocket is routed here, 
indexed by symbol.
*/
export const actionStore = createStore(
	{
		actions: {} as Record<string, ReturnType<typeof Circular<Action>>>,
	},
	({ setState }) => ({
		updateFrame: (frames: Action[]) =>
			setState((prev) => {
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
