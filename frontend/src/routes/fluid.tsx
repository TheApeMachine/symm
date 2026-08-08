import { createFileRoute } from "@tanstack/react-router";
import { FluidInspector } from "#/components/fluid-3d";

const RouteComponent = () => <FluidInspector />;

export const Route = createFileRoute("/fluid")({
	component: RouteComponent,
});
