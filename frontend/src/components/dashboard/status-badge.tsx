/*
StatusBadge is the single small-status-pill style used across the
terminal: a color-mix border and fill tinted by tone, with the text
kept in the same solid tone. Every status word (online/offline,
decisive/ambiguous, awake/sideline, thin/nominal, ...) should render
through this component so they share one size and one border/background
treatment, and only the tone itself varies.
*/
export const StatusBadge = ({
	label,
	tone,
}: {
	label: string;
	tone: string;
}) => (
	<span
		className="inline-flex items-center rounded-[2px] border px-[7px] py-0.5 font-semibold text-[10px] uppercase tracking-[0.08em]"
		style={{
			borderColor: `color-mix(in srgb, ${tone} 40%, transparent)`,
			backgroundColor: `color-mix(in srgb, ${tone} 12%, transparent)`,
			color: tone,
		}}
	>
		{label}
	</span>
);
