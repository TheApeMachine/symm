

export const ContextStrip = ({
	label,
	symbols,
	meta,
	activeSymbol,
	onSelect,
}: {
	label: string;
	symbols: string[];
	meta?: string;
	activeSymbol?: string;
	onSelect?: (symbol: string) => void;
}) => (
	<div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
		<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
			{label}
		</span>
		{symbols.map((symbol) => {
			const active = activeSymbol === symbol;

			return (
				<button
					key={symbol}
					type="button"
					onClick={() => onSelect?.(symbol)}
					className="shrink-0 cursor-pointer rounded-[3px] border px-[11px] py-1 font-mono text-[11px] font-medium"
					style={{
						borderColor: active ? "var(--acc)" : "var(--line)",
						background: active
							? "color-mix(in srgb, var(--acc) 14%, transparent)"
							: "transparent",
						color: active ? "var(--acc)" : "var(--f3)",
					}}
				>
					{symbol.split("/")[0] ?? symbol}
				</button>
			);
		})}
		{meta ? (
			<span className="ml-auto shrink-0 font-mono text-[10px] text-(--f4)">
				{meta}
			</span>
		) : null}
	</div>
);