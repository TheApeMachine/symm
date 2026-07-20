import type { ReactNode } from "react";
import { LifecycleTrack } from "#/components/terminal/lifecycle-track";
import { Panel } from "@/components/ui/panel";

const Section = ({
	title,
	metaRole,
	emptyRole,
	empty,
	listRole,
	children,
}: {
	title: string;
	metaRole: string;
	emptyRole: string;
	empty: string;
	listRole?: string;
	children?: ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<div className="mb-2 flex items-center justify-between gap-2">
			<span className="font-semibold text-[11px] text-(--f1) uppercase tracking-[0.08em]">
				{title}
			</span>
			<span
				data-thesis={metaRole}
				className="font-mono text-[9px] text-(--f4)"
			/>
		</div>
		{children}
		{empty === "" ? null : (
			<div
				data-thesis={emptyRole}
				className="font-mono text-[10px] text-(--f4)"
			>
				{empty}
			</div>
		)}
		{listRole === undefined ? null : (
			<div data-thesis={listRole} className="flex flex-col gap-1.5" />
		)}
	</Panel>
);

/*
ThesisDetailRail is the static thesis carrier shell. paintThesis writes it.
*/
export const ThesisDetailRail = () => (
	<div
		data-thesis="rail"
		className="flex min-h-0 flex-col gap-2 overflow-auto pr-1"
	>
		<Section
			title="Lifecycle"
			metaRole="lifecycle-meta"
			emptyRole="lifecycle-empty"
			empty=""
		>
			<LifecycleTrack symbol="" state="observing" />
		</Section>

		<Section
			title="Decision"
			metaRole="decision-meta"
			emptyRole="decision-empty"
			empty="no strategy decision retained for this symbol"
		>
			<div
				data-thesis="decision-body"
				className="flex flex-col gap-1 font-mono text-[10px] text-(--f3)"
				style={{ display: "none" }}
			>
				<div>
					utility{" "}
					<span data-thesis="decision-utility" className="text-(--f1)" />
				</div>
				<div>
					proposed{" "}
					<span data-thesis="decision-proposed" className="text-(--acc)" />
				</div>
				<div>
					return{" "}
					<span data-thesis="decision-return" className="text-(--f1)" />
					{" · conf "}
					<span data-thesis="decision-confidence" className="text-(--f1)" />
				</div>
				<div data-thesis="decision-cause" className="text-(--f4)" />
			</div>
		</Section>

		<Section
			title="Forecasts"
			metaRole="forecasts-meta"
			emptyRole="forecasts-empty"
			empty="none published"
			listRole="forecasts-list"
		/>
		<Section
			title="Hypotheses"
			metaRole="hypotheses-meta"
			emptyRole="hypotheses-empty"
			empty="none published"
			listRole="hypotheses-list"
		/>
		<Section
			title="Categories"
			metaRole="categories-meta"
			emptyRole="categories-empty"
			empty="none published"
			listRole="categories-list"
		/>
		<Section
			title="Holdings"
			metaRole="holdings-meta"
			emptyRole="holdings-empty"
			empty="no holdings"
			listRole="holdings-list"
		/>
		<Section
			title="Findings"
			metaRole="findings-meta"
			emptyRole="findings-empty"
			empty="none retained · postmortem after exit"
			listRole="findings-list"
		/>
	</div>
);
