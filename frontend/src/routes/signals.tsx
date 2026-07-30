import { createFileRoute } from "@tanstack/react-router";
import { SignalsSurface } from "#/components/terminal/signals-surface";

const RouteComponent = () => {
	return <SignalsSurface />;
};

export const Route = createFileRoute("/signals")({
	component: RouteComponent,
});
