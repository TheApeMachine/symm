import { Panel } from "@/components/ui/panel";

export const AllocationMain = ({ symbols }: { symbols: string[] }) => (
	<div className="min-h-0 overflow-auto px-[18px] py-4">
		<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
			<span className="text-(--f3)">cross-section</span>
			<span className="text-(--f4)">
				median <span data-alloc="median" className="text-(--f2)" />
			</span>
			<span className="text-(--f4)">
				mad <span data-alloc="mad" className="text-(--f2)" />
			</span>
			<span className="text-(--f4)">
				entry <span data-alloc="threshold" className="text-(--acc)" />
			</span>
			<span className="ml-auto text-(--f4)">live ladder frames</span>
		</div>

		<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
			<span className="w-[58px] shrink-0">symbol</span>
			<span className="flex-1">thesis score {"->"} sqrt(confirmations)</span>
			<span className="w-[52px] shrink-0 text-right">edge</span>
			<span className="w-[42px] shrink-0 text-right">share</span>
			<span className="w-[74px] shrink-0 text-right">notional</span>
		</div>

		<div className="flex flex-col">
			<div
				data-alloc="waiting"
				className="py-24 text-center font-mono text-[11px] text-(--f4)"
			>
				waiting for backend decision frames
			</div>

			{symbols.map((symbol) => (
				<div
					key={symbol}
					data-alloc-row={symbol}
					data-symbol={symbol}
					className="flex items-center gap-[9px] border-(--line) border-b py-[7px]"
					style={{ display: "none" }}
				>
					<span
						data-alloc="name"
						className="w-[58px] shrink-0 font-mono text-[11px] font-semibold"
					>
						{symbol.split("/")[0]}
					</span>
					<div className="relative h-[18px] flex-1">
						<div className="absolute top-2 right-0 left-0 h-px bg-(--line)" />
						<div
							data-alloc="median-mark"
							className="absolute top-px bottom-px w-px bg-(--f4)"
						/>
						<div
							data-alloc="threshold-mark"
							className="absolute top-0 bottom-0 w-px bg-[color-mix(in_srgb,var(--acc)_70%,transparent)]"
						/>
						<div
							data-alloc="edge-bar"
							className="absolute top-[7px] h-[3px] rounded-sm bg-(--acc)"
						/>
						<div
							data-alloc="dot"
							className="absolute top-1 h-[9px] w-[9px] rounded-full border border-(--sunken)"
							style={{ marginLeft: "-4.5px" }}
						/>
					</div>
					<span
						data-alloc="edge"
						className="w-[52px] shrink-0 text-right font-mono text-[10px]"
					/>
					<span
						data-alloc="share"
						className="w-[42px] shrink-0 text-right font-mono text-[10px] text-(--f2)"
					/>
					<span
						data-alloc="notional"
						className="w-[74px] shrink-0 text-right font-mono text-[10.5px]"
					/>
				</div>
			))}
		</div>

		<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
			{[
				["var(--acc)", "allocated"],
				["var(--info)", "in play · below edge"],
				["var(--f4)", "scanned"],
			].map(([color, label]) => (
				<span key={label} className="inline-flex items-center gap-[5px]">
					<span
						className="h-2 w-2 rounded-full"
						style={{ background: color }}
					/>
					{label}
				</span>
			))}
			<span className="inline-flex items-center gap-[5px]">
				<span className="h-px w-[10px] bg-(--acc)" /> entry line
			</span>
		</div>
	</div>
);

export const AllocationSidePanel = ({ symbols }: { symbols: string[] }) => (
	<div className="flex flex-col gap-3.5">
		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Capital deployment
				</span>
				<span data-alloc="deployed-pct" className="font-mono text-[11px] text-(--acc)" />
			</div>
			<div className="mt-1 mb-[11px] font-mono text-[9.5px] text-(--f4)">
				share of deployable free cash
			</div>
			<div className="h-2 overflow-hidden rounded-[2px] bg-(--line)">
				<div
					data-alloc="deploy-fill"
					className="h-full bg-(--acc)"
					style={{ width: "0%" }}
				/>
			</div>
			<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
				<span data-alloc="deployed-label" />
				<span data-alloc="reserved-label" />
			</div>
		</Panel>

		<Panel>
			<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
				Position sizing
			</div>
			<div
				data-alloc="quote-label"
				className="mb-[11px] font-mono text-[9.5px] text-(--f4)"
			/>
			<div className="flex flex-col gap-[9px]">
				<div
					data-alloc="sizing-empty"
					className="border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4)"
				>
					no symbols above entry edge
				</div>
				{symbols.map((symbol) => (
					<div
						key={symbol}
						data-alloc-size={symbol}
						data-symbol={symbol}
						style={{ display: "none" }}
					>
						<div className="mb-1 flex items-center justify-between">
							<span className="font-mono text-[11px] font-semibold text-(--f1)">
								{symbol.split("/")[0]}
							</span>
							<span
								data-alloc="size-notional"
								className="font-mono text-[10.5px] text-(--acc)"
							/>
						</div>
						<div className="flex items-center gap-2">
							<div className="h-1.5 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
								<div
									data-alloc="size-fill"
									className="h-full bg-(--acc)"
									style={{ width: "0%" }}
								/>
							</div>
							<span
								data-alloc="size-share"
								className="w-10 shrink-0 text-right font-mono text-[10px] text-(--f2)"
							/>
						</div>
					</div>
				))}
			</div>
		</Panel>
	</div>
);
