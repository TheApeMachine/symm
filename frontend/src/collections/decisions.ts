import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

export const decisionStore = createStore(
	{
		tick: null as number | null,
		decisions: Circular(50),
		allowed: [] as Record<string, unknown>[],
		denied: [] as Record<string, unknown>[],
	},
	({ setState }) => ({
		observeTick: (tick: unknown) =>
			setState((prev) => {
				const count = Number(tick);

				if (!Number.isFinite(count) || prev.tick === count) {
					return prev;
				}

				return { ...prev, tick: count };
			}),
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const next = { ...prev };
				const decisions = Array.isArray(frame)
					? (frame as Record<string, unknown>[])
					: [frame as Record<string, unknown>];

				for (const decision of decisions) {
					const tick = Number(decision.tick);

					if (Number.isFinite(tick)) {
						next.tick = tick;
					}

					next.decisions.push(decision);
					next.allowed =
						decision.verdict === "allow"
							? [...next.allowed, decision].slice(-50)
							: next.allowed;
					next.denied =
						decision.verdict === "allow"
							? next.denied
							: [...next.denied, decision].slice(-50);
				}

				return {
					...next,
				};
			}),
		reset: () =>
			setState(() => ({
				tick: null,
				decisions: Circular(50),
				allowed: [],
				denied: [],
			})),
	}),
);
