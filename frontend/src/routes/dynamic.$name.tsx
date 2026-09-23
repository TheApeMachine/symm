import { createFileRoute } from "@tanstack/react-router";
import { GraphSurface } from "#/components/surface/graph-surface";

/*
Draws a graph by name regardless of what path it declares, so a surface being
rebuilt can be watched at /dynamic/<name> while the React one it replaces is
still standing at the real URL.
*/
const DynamicGraphRoute = () => {
	const { name } = Route.useParams();

	return <GraphSurface name={name} />;
};

export const Route = createFileRoute("/dynamic/$name")({
	component: DynamicGraphRoute,
});
