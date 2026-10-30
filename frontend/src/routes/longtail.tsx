import { createFileRoute } from "@tanstack/react-router";
import { LongtailDashboard } from "#/components/longtail/dashboard";

export const Route = createFileRoute("/longtail")({
	component: RouteComponent,
});

function RouteComponent() {
	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden">
			<LongtailDashboard />
		</div>
	);
}
