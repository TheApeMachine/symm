import { createFileRoute } from "@tanstack/react-router";
import { useMemo } from "react";
import { usePipelineGraphRow } from "#/collections/pipeline_graph_row";
import { compileUI } from "#/components/ui/compiler";
import { renderUIRoute } from "#/components/ui/renderer";

const LOCAL_GRAPH_ID = "local-default";

/*
UIGraphRouteComponent compiles the active Flume UI graph into a runtime route tree
and renders it with the generic recursive renderer.
*/
function UIGraphRouteComponent() {
	const row = usePipelineGraphRow(LOCAL_GRAPH_ID);

	const compilation = useMemo(() => {
		if (!row?.nodes || Object.keys(row.nodes).length === 0) {
			return null;
		}

		return compileUI({ nodes: row.nodes as any });
	}, [row?.nodes]);

	if (!compilation || compilation.routes.length === 0) {
		return (
			<div className="flex h-full w-full flex-col items-center justify-center p-8 text-neutral-400 font-mono text-xs gap-2">
				<div>Compiled UI Graph shell ready</div>
				<div className="text-neutral-500">
					Author a route using `ui.UIRoute` or structural components in the
					graph editor.
				</div>
			</div>
		);
	}

	const primaryRoute = compilation.routes[0];

	return (
		<div className="relative h-full w-full">
			{compilation.diagnostics.length > 0 && (
				<div className="absolute top-2 right-2 z-50 max-w-md bg-red-950/80 border border-red-500/50 p-3 rounded text-red-200 text-xs font-mono backdrop-blur">
					<div className="font-bold mb-1">
						UI Graph Compilation Warnings ({compilation.diagnostics.length}):
					</div>
					<ul className="list-disc pl-4 space-y-1">
						{compilation.diagnostics.map((d, i) => (
							<li key={`${d.kind}-${d.nodeId ?? i}-${d.propName ?? ""}`}>
								{d.message}
							</li>
						))}
					</ul>
				</div>
			)}
			{renderUIRoute(primaryRoute)}
		</div>
	);
}

export const Route = createFileRoute("/ui-graph")({
	component: UIGraphRouteComponent,
});
