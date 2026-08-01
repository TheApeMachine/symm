import { DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";
import { Component } from "../ui/component";

const interactive = (compact: boolean, source: string) => {
	if (compact) {
		return () => {
			terminalStore.actions.selectSource(source);
		};
	}

	return () => {
		terminalStore.actions.inspectSource(source);
	};
};

/*
KernelList binds each stable row directly to the raw measurements stream.
Rows select their matching frame via data-scope/data-filter and then paint
only the fields they need from that frame.
*/
export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => (
	<Component registerKey="measurements">
		{({ ref, className }) => (
			<div ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				{sources.map((source) => (
					<button
						key={source}
						type="button"
						data-scope="source"
						data-filter={source}
						onClick={interactive(compact, source)}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
					>
						<div className="flex items-center justify-between gap-2">
							<span
								data-paint="source"
								className={cn("truncate font-semibold text-(--f1)", {
									"text-xs": compact,
									"text-[12.5px]": !compact,
								})}
							>
								{source}
							</span>

							{compact ? (
								<span
									data-paint="validity.state"
									data-paint-class="valid:bg-(--up) invalid:bg-(--down) provisional:bg-(--info)"
									className="size-1.75 shrink-0 overflow-hidden rounded-full bg-(--f3) text-[0] leading-none"
								>
									STANDBY
								</span>
							) : (
								<span
									data-paint="validity.state"
									data-paint-class="valid:border-[color-mix(in_srgb,var(--up)_38%,transparent)],bg-[color-mix(in_srgb,var(--up)_12%,transparent)],text-(--up) invalid:border-[color-mix(in_srgb,var(--down)_38%,transparent)],bg-[color-mix(in_srgb,var(--down)_12%,transparent)],text-(--down) provisional:border-[color-mix(in_srgb,var(--info)_38%,transparent)],bg-[color-mix(in_srgb,var(--info)_12%,transparent)],text-(--info)"
									className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em] text-(--f3)"
								>
									STANDBY
								</span>
							)}
						</div>

						<div className="mt-0.5 truncate font-mono text-[9.5px] text-(--f4)">
							<span data-paint="symbol" className="text-(--f4)">
								waiting
							</span>
						</div>

						{compact ? null : (
							<>
								<svg
									viewBox="0 0 150 30"
									preserveAspectRatio="none"
									className="mt-1.5 block h-6.5 w-full"
								>
									<title>Signal trace</title>
								</svg>

								<div className="mt-1.5 flex items-center gap-2">
									<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)">
										<div className="h-full w-0 bg-(--warning)" />
									</div>

									<span
										data-paint="validity.readiness"
										className="w-16 shrink-0 truncate text-right font-mono text-[9.5px] uppercase text-(--f4)"
									/>

									<span
										data-paint="at"
										className="w-28 shrink-0 truncate text-right font-mono text-[9.5px] text-(--f4)"
									/>
								</div>
							</>
						)}
					</button>
				))}
			</div>
		)}
	</Component>
);

export const paintKernelList = <T,>(updates: T, _focusSymbol?: string): T =>
	updates;
