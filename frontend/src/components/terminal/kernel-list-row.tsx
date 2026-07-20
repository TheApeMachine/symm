/*
KernelListRow is static row chrome. KernelList owns the row element ref and
paints live fields under data-role markers.
*/
export const KernelListRow = ({
	source,
	compact = false,
	rowRef,
	onActivate,
}: {
	source: string;
	compact?: boolean;
	rowRef: (element: HTMLButtonElement | null) => void;
	onActivate: (source: string) => void;
}) => (
	<button
		ref={rowRef}
		type="button"
		onClick={() => onActivate(source)}
		className="block w-full cursor-pointer border-(--line) border-b border-l-2 px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
		style={{
			borderLeftColor: "transparent",
			background: "transparent",
		}}
	>
		<div className="flex items-center justify-between gap-2">
			<span
				className={`truncate font-semibold text-(--f1) ${
					compact ? "text-xs" : "text-[12.5px]"
				}`}
			>
				{source}
			</span>

			{compact ? (
				<span
					data-role="status"
					className="size-[7px] shrink-0 rounded-full bg-(--f3)"
				/>
			) : (
				<span
					data-role="status"
					className="shrink-0 rounded-[2px] border border-(--line2) bg-(--line) px-[5px] py-0.5 font-mono text-[9px] uppercase tracking-[0.07em] text-(--f3)"
				>
					Standby
				</span>
			)}
		</div>

		{compact ? null : (
			<>
				<svg
					viewBox="0 0 150 30"
					preserveAspectRatio="none"
					className="mt-1.5 block h-[26px] w-full"
				>
					<title>Signal sparkline</title>
					<polyline
						data-role="spark-area"
						fill="color-mix(in srgb, var(--info) 12%, transparent)"
						stroke="none"
					/>
					<polyline
						data-role="spark-line"
						fill="none"
						stroke="var(--info)"
						strokeWidth="1.4"
						vectorEffect="non-scaling-stroke"
					/>
				</svg>

				<div className="mt-1.5 flex items-center gap-2">
					<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
						<div
							data-role="bar"
							className="h-full transition-[width] duration-500 ease-out"
							style={{ width: "0%" }}
						/>
					</div>

					<span
						data-role="readout"
						className="flex-1 truncate text-right font-mono text-[10px] text-(--f2)"
					/>

					<span
						data-role="age"
						className="w-[46px] shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
					/>
				</div>
			</>
		)}

		{compact ? (
			<div
				data-role="readout"
				className="mt-1 truncate font-mono text-[9px] text-(--f4)"
			/>
		) : null}
	</button>
);
