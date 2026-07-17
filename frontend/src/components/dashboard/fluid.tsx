import { Flex } from "@/components/ui/flex";

export const FluidLegend = () => (
	<Flex.Row className="pointer-events-none absolute bottom-2.5 left-3 gap-3.5 font-mono text-[9px] text-(--f3)">
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			particle
		</Flex.Row>
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-info" />
			guidance current
		</Flex.Row>
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-(--down)" />
			gas ρ
		</Flex.Row>
	</Flex.Row>
);
