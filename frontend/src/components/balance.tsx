import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
Balance is the account readout.

Cash alone understates the account while lots are open, so the unrealized
result sits beside it and equity states what the balance would settle at if
everything were closed now. All three come from one frame, which keeps them
describing the same instant.

Each figure is a painted span, not a React value: the frame writes the digits
into the node directly, so a balance that moves on every tick costs no render.
*/

const Reading = ({
	label,
	bind,
	tone,
	weight,
}: {
	label: string;
	bind: string;
	tone: "f1" | "f2" | "accent";
	weight: "medium" | "semibold";
}) => (
	<Flex.Column className="items-end gap-px">
		<Typography.Label size="s" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono
			size="lg"
			tone={tone}
			weight={weight}
			data-paint={bind}
			data-paint-format=".2f"
		/>
	</Flex.Column>
);

export const Balance = () => {
	return (
		<Component registerKey="equity">
			{({ ref, className }) => (
				<Flex.Row ref={ref} align="center" gap={6} className={cn(className)}>
					<Reading label="Cash" bind="cash" tone="f1" weight="medium" />
					<Reading
						label="Unrealized"
						bind="unrealized"
						tone="f2"
						weight="medium"
					/>
					<Reading
						label="Equity"
						bind="equity"
						tone="accent"
						weight="semibold"
					/>
				</Flex.Row>
			)}
		</Component>
	);
};
