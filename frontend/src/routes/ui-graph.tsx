import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { type CompiledUIRoute, renderUIRoute } from "#/components/ui/renderer";

/*
UIGraphRouteComponent provides a stable dynamic shell in TanStack Router
to render compiled UI route graphs.
*/
function UIGraphRouteComponent() {
	const [route] = useState<CompiledUIRoute | null>(null);

	if (!route) {
		return (
			<div className="flex h-full w-full items-center justify-center p-8 text-neutral-400 font-mono text-xs">
				Compiled UI Graph shell ready
			</div>
		);
	}

	return renderUIRoute(route);
}

export const Route = createFileRoute("/ui-graph")({
	component: UIGraphRouteComponent,
});
