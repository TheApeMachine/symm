import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

export const Clock = () => {
	return (
		<Component registerKey="tick">
			{({ ref, className }) => (
				<Flex.Column ref={ref} className={cn(className)}>
					<Flex>clock</Flex>
					<Flex>uptime -</Flex>
				</Flex.Column>
			)}
		</Component>
	);
};
