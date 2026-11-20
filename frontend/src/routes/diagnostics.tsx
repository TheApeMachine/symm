import { createFileRoute } from "@tanstack/react-router";
import { DiagnosticsGraph } from "#/components/diagnostics/graph";

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsPage,
});

function DiagnosticsPage() {
	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="border-(--line) border-b px-4 py-2">
				<div className="font-mono text-[10px] uppercase tracking-[0.2em] text-(--f3)">
					System diagnostics
				</div>
				<div className="mt-0.5 font-mono text-[11px] text-(--f4)">
					Pipeline topology · stage health · work time
				</div>
			</div>
			<div className="min-h-0 flex-1">
				<DiagnosticsGraph />
			</div>
		</div>
	);
}
