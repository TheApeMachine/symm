import { createFileRoute } from "@tanstack/react-router";
import { GraphSurface } from "#/components/terminal/graph-surface";

const RouteComponent = () => {
	return <GraphSurface />;
};

export const Route = createFileRoute("/graph")({
	component: RouteComponent,
});
