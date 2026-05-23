interface Props {
	label: string;
	value: string;
	tone?: string;
}

export const Metric = ({ label, value, tone }: Props) => {
	return (
		<div className="hidden flex-col sm:flex">
			<span className="text-[10px] uppercase tracking-wide text-(--dash-muted)">
				{label}
			</span>
			<span className={`text-xs font-medium tabular-nums ${tone ?? ""}`}>
				{value}
			</span>
		</div>
	);
};
