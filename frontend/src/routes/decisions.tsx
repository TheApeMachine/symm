import { createFileRoute } from "@tanstack/react-router";
import { DecisionsSurface } from "#/components/terminal/decisions-surface";

export const Route = createFileRoute("/decisions")({
	component: DecisionsSurface,
});
