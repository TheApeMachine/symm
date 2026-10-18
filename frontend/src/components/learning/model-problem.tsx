import { useSelector } from "@tanstack/react-store";
import { cognitionStore } from "#/collections/cognition";
import { Alert } from "#/components/ui/alert";

/*
ModelProblem is the banner for why make run has no model to trade: the
problem the hub's retained cognition/tree frame carries, at its level, with
the exact reason and the fix. Nothing when the loaded tree is usable or no
tree frame has arrived yet.
*/
export const ModelProblem = ({ size = "m" }: { size?: "s" | "m" }) => {
	const problem = useSelector(
		cognitionStore,
		(state) => state.problem ?? state.tree?.problem,
	);

	if (!problem) return null;

	return (
		<Alert variant={problem.level} size={size} data-problem={problem.level}>
			<span className="font-medium">
				paper trading inactive
				{problem.level === "error" ? " (model error)" : ""}:
			</span>{" "}
			{problem.reason}. <span className="font-medium">Fix:</span> {problem.fix}.
		</Alert>
	);
};
