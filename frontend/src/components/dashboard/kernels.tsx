import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

export const KernelList = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);

	return (
		<Component registerKey="measurements">
			{({ ref, className }) => (
				<List
					ref={ref}
					className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
				>
					{kernels.map((kernel, index) => (
						<List.Item
							key={kernel}
							data-index={index}
							className="block border-(--line) border-b px-3 py-2.5"
						>
							<Flex.Row className="items-center justify-between gap-2">
								<Typography.Span
									data-paint="source"
									className="truncate font-semibold text-[12.5px] text-(--f1)"
								>
									{kernel}
								</Typography.Span>
								<Typography.Span
									data-paint="status"
									data-paint-class="HEALTHY:text-(--up) INVALID:text-(--down) STANDBY:text-(--f4)"
									className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em]"
								>
									STANDBY
								</Typography.Span>
							</Flex.Row>
							<div className="mt-2 h-1 overflow-hidden rounded-xs bg-(--line)">
								<div
									data-set="bar_width"
									data-target="style.width"
									className="h-full w-0 bg-[color-mix(in_srgb,var(--warning)_82%,transparent)]"
								/>
							</div>
							<Flex.Row className="mt-1.5 items-center gap-2">
								<Typography.Span
									data-paint="readout"
									className="min-w-0 flex-1 truncate font-mono text-[10px] text-(--f2)"
								>
									waiting
								</Typography.Span>
								<Typography.Span
									data-paint="age"
									className="w-11.5 shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
								/>
							</Flex.Row>
						</List.Item>
					))}
				</List>
			)}
		</Component>
	);
};
