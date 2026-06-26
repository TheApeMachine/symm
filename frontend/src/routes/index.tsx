import { createFileRoute } from "@tanstack/react-router";
import { DashboardSurface } from "#/components/terminal/dashboard";

const RouteComponent = () => <DashboardSurface />;

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
