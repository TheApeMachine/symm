import { Flex } from "@/components/ui/flex";

export const FluidLegend = () => (
	<Flex.Row className="pointer-events-none absolute bottom-2.5 left-3 gap-3.5 font-mono text-[9px] text-(--f3)">
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			whale carrier
		</Flex.Row>
		<Flex.Row align="center" className="gap-1.5">
			<span className="inline-block h-px w-3 bg-[rgba(255,228,180,0.75)]" />
			guidance flow
		</Flex.Row>
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-info/70" />
			|psi|^2 veil
		</Flex.Row>
	</Flex.Row>
);
