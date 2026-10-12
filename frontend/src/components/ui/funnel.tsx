import { cn } from "#/lib/utils";

export interface FunnelStage {
	label: string;
	/* Segments sum to the stage's size, each with its own colour. */
	segments: Array<{ value: number; color: string; label: string }>;
}

export interface FunnelProps {
	/* Size of the full population the stages narrow from. */
	total: number;
	stages: FunnelStage[];
	className?: string;
}

/*
Funnel draws how a population narrows stage by stage: every row is centred
and as wide as what is left, split into coloured segments by kind.
*/
export const Funnel = ({ total, stages, className }: FunnelProps) => (
	<div className={cn("flex flex-col gap-1", className)}>
		{stages.length === 0 && <span className="text-[10px] text-(--f4)">nothing to narrow yet</span>}
		{stages.map((stage) => {
			const size = stage.segments.reduce((a, s) => a + s.value, 0);
			const width = total > 0 ? (size / total) * 100 : 0;
			return (
				<div key={stage.label} className="flex items-center gap-2">
					<span className="w-6 text-right text-[9px] text-(--f4) font-mono">{stage.label}</span>
					<div className="flex-1 flex justify-center h-3.5 bg-(--sunken) rounded-sm">
						{size === 0 ? (
							<div className="w-px bg-(--down)" />
						) : (
							<div className="flex h-full rounded-sm overflow-hidden" style={{ width: `${Math.max(width, 2)}%`, transition: "width 300ms" }}>
								{stage.segments.map((seg) =>
									seg.value > 0 ? <div key={seg.label} title={`${seg.value} ${seg.label}`} style={{ flex: seg.value, background: seg.color }} /> : null,
								)}
							</div>
						)}
					</div>
					<span className="w-6 text-[9px] text-(--f3) font-mono">{size}</span>
				</div>
			);
		})}
	</div>
);
