import { createFileRoute } from "@tanstack/react-router";

const RouteComponent = () => {
	// The cortex / cognitive prefix-tree surface has no backend feed yet: no
	// signal emits a "cognitive" origin and the dmt.Tree cognitive engine is not
	// published to the ui broadcast. Rather than fabricate a beam tree, this
	// surface stays honestly empty until the backend emits cognitive frames.
	return (
		<div className="flex h-full min-w-[1140px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Cognitive tree
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
						sensory prefix tree · beam search lookahead · attractor basin
					</div>
				</div>
			</div>
			<div className="flex flex-1 items-center justify-center font-mono text-[11px] text-(--f4)">
				no cognitive frames — backend does not publish this surface yet
			</div>
		</div>
	);
};

export const Route = createFileRoute("/cortex")({
	component: RouteComponent,
});
