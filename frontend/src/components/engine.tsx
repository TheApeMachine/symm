import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

export const Engine = () => {
	return (
		<Component registerKey="tick">
			{({ ref, className }) => (
				<Flex.Column
					ref={ref}
					className={cn(
						"mx-2 border border-(--line) rounded-[3px] bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]",
						className,
					)}
				>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">seq</Flex>
						<Flex data-paint="count" className="text-(--f1)" />
					</Flex.Row>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">phase</Flex>
						<Flex data-paint="phase" className="text-(--acc)" />
					</Flex.Row>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">cand</Flex>
						<Flex data-paint="candidates" className="text-(--f1)" />
					</Flex.Row>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">open</Flex>
						<Flex data-paint="open" className="text-(--f1)" />
					</Flex.Row>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">meas</Flex>
						<Flex data-paint="measurements" className="text-(--f1)" />
					</Flex.Row>
					<Flex.Row justify="between">
						<Flex className="text-(--f4)">ready</Flex>
						<Flex.Row gap={1}>
							<Flex data-paint="quotes_ready" className="text-(--f1)" />
							<Flex className="text-(--f1)">/</Flex>
							<Flex data-paint="quotes_total" className="text-(--f1)" />
						</Flex.Row>
					</Flex.Row>
				</Flex.Column>
			)}
		</Component>
	);
};
