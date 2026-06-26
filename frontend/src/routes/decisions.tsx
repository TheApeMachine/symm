import { createFileRoute } from "@tanstack/react-router";
import { DecisionTreeView } from "#/components/terminal/decision";

const RouteComponent = () => <DecisionTreeView />;

export const Route = createFileRoute("/decisions")({
	component: RouteComponent,
});
