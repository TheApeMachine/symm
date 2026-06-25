export const Fact = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f3)">{label}</span>
		<span
			className="truncate"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</span>
	</div>
);
