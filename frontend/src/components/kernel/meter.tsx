export const InspectorMeter = ({
	label,
	value,
	percent,
	color,
}: {
	label: string;
	value: string;
	percent: number;
	color: string;
}) => (
	<div>
		<div className="mb-1 flex justify-between font-mono text-[10px]">
			<span className="text-(--f3)">{label}</span>
			<span className="text-(--f1)">{value}</span>
		</div>
		<div className="h-1 overflow-hidden rounded-[2px] bg-(--line)">
			<div
				className="h-full"
				style={{ width: `${percent}%`, background: color }}
			/>
		</div>
	</div>
);
