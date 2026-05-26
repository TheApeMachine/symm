export const SidebarSection = ({
	title,
	children,
	className = "",
	fill = false,
}: {
	title: string;
	children: React.ReactNode;
	className?: string;
	fill?: boolean;
}) => {
	return (
		<section
			className={`${fill ? "flex min-h-0 flex-1 flex-col" : ""} ${className}`}
		>
			<h2 className="px-3 py-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-(--dash-muted)">
				{title}
			</h2>
			<div className={fill ? "min-h-0 flex-1 overflow-auto" : ""}>
				{children}
			</div>
		</section>
	);
};
