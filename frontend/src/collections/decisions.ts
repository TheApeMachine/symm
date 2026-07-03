import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

export const decisionStore = createStore(
	{
		decisions: Circular(50),
		allowed: [] as Record<string, unknown>[],
		denied: [] as Record<string, unknown>[],
	},
	({ setState }) => ({
		updateFrame: (decision: Record<string, unknown>) =>
			setState((prev) => {
				prev.decisions.push(decision);

				return {
					...prev,
					allowed:
						decision.verdict === "allow"
							? [...prev.allowed, decision].slice(-50)
							: prev.allowed,
					denied:
						decision.verdict === "allow"
							? prev.denied
							: [...prev.denied, decision].slice(-50),
				};
			}),
		reset: () =>
			setState(() => ({
				decisions: Circular(50),
				allowed: [],
				denied: [],
			})),
	}),
);
