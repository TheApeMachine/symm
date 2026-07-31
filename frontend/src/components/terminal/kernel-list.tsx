import { DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Component } from "../ui/component";

/*
KernelList mounts one stable row per source and lets Component paint direct
value updates in place while paintKernelList keeps the sparkline incremental.
*/
export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => (
	<Component register={(paint) => registerPainter("measurements", paint)}>
		{({ ref, className }) => (
			<div ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				{sources.map((source, index) => (
					<button
						key={source}
						type="button"
						data-scope="source"
						data-filter={source}
						data-index={index}
						onClick={() => {
							if (compact) {
								terminalStore.actions.selectSource(source);
								return;
							}
						}}
						ref={(element) => {
							if (element === null) {
								return;
							}
						}}
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
									data-set="statusDot"
									data-target="style.backgroundColor"
									className="size-1.75 shrink-0 rounded-full bg-(--f3)"
								/>
							) : (
								<span
									data-paint="status"
									data-paint-class="HEALTHY:border-[color-mix(in_srgb,var(--up)_38%,transparent)],bg-[color-mix(in_srgb,var(--up)_12%,transparent)],text-(--up) INVALID:border-[color-mix(in_srgb,var(--down)_38%,transparent)],bg-[color-mix(in_srgb,var(--down)_12%,transparent)],text-(--down) STANDBY:border-(--line2),bg-(--line),text-(--f3)"
									className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em] text-(--f3)"
								>
									STANDBY
								</span>
							)}
						</div>

						{compact ? null : (
							<>
								<svg
									viewBox="0 0 150 30"
									preserveAspectRatio="none"
									className="mt-1.5 block h-6.5 w-full"
								>
									<title>Signal sparkline</title>
									<polyline
										data-role="spark-area"
										className="fill-[color-mix(in_srgb,var(--acc)_16%,transparent)]"
										stroke="none"
									/>
									<polyline
										data-role="spark-line"
										className="stroke-(--acc)"
										fill="none"
										strokeWidth="1.4"
										vectorEffect="non-scaling-stroke"
									/>
								</svg>

								<div className="mt-1.5 flex items-center gap-2">
									<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)">
										<div
											data-set="barWidth"
											data-target="style.width"
											className="h-full w-0 bg-(--warning) transition-[width] duration-500 ease-out"
										/>
									</div>

									<span
										data-paint="readout"
										className="flex-1 truncate text-right font-mono text-[10px] text-(--f2)"
									>
										waiting
									</span>

									<span
										data-paint="age"
										className="w-11.5 shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
									/>
								</div>
							</>
						)}

						{compact ? (
							<div
								data-paint="readout"
								className="mt-1 truncate font-mono text-[9px] text-(--f4)"
							>
								waiting
							</div>
						) : null}
					</button>
				))}
			</div>
		)}
	</Component>
);
