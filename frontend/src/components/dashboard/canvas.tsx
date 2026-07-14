import type { ReactNode } from "react";

export const Canvas = ({
	title,
	meta,
	topRight,
	legend,
	footer,
	children,
	className,
}: {
	title: ReactNode;
	meta: string;
	topRight?: ReactNode;
	legend?: ReactNode;
	footer?: ReactNode;
	children: ReactNode;
	className: string;
}) => (
	<div className={`relative min-h-0 overflow-hidden bg-[#0a0907] ${className}`}>
		<div className="absolute inset-0">{children}</div>
		<div className="pointer-events-none absolute inset-0 opacity-50 bg-[repeating-linear-gradient(0deg,rgba(0,0,0,0.18)_0px,rgba(0,0,0,0.18)_1px,transparent_1px,transparent_3px)] mix-blend-multiply" />
		<div className="pointer-events-none absolute top-[11px] left-3">
			<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
				{title}
			</div>
			<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">{meta}</div>
		</div>
		{topRight ? (
			<div className="pointer-events-none absolute top-[11px] right-3 text-right font-mono text-[9.5px] text-(--f3) leading-[1.6]">
				{topRight}
			</div>
		) : null}
		{legend}
		{footer ? (
			<div className="pointer-events-none absolute right-3 bottom-2 font-mono text-[9.5px] text-(--f3)">
				{footer}
			</div>
		) : null}
	</div>
);
