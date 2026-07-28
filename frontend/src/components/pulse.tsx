import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";

export const Pulse = () => {
	return (
		<Component
			className="metric-grid"
			register={(paint) => registerPainter("tick", paint)}
		>
			{({ ref, className }) => (
				<Flex.Row
					ref={ref}
					align="center"
					gap={4}
					className={cn(
						"h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)",
						className,
					)}
				>
					<span data-paint="count" className="font-semibold text-(--f1)" />
					<span>
						phase <span data-paint="phase" className="text-(--acc)" />
					</span>
					<span>
						meas <span data-paint="measurements" />
					</span>
					<span>
						cand <span data-paint="candidates" />
					</span>
					<span>
						open <span data-paint="open" />
					</span>
					<span>
						ready <span data-paint="quotes_ready" />/
						<span data-paint="quotes_total" />
					</span>
				</Flex.Row>
			)}
		</Component>
	);
};
