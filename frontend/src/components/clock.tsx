import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";

export const Clock = () => {
	return (
		<Component register={(paint) => registerPainter("tick", paint)}>
			{({ ref, className }) => (
				<Flex.Column ref={ref} className={cn(className)}>
					<Flex>clock</Flex>
					<Flex>uptime -</Flex>
				</Flex.Column>
			)}
		</Component>
	);
};
