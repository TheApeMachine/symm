import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

/*
Balance is the account readout.

Cash alone understates the account while lots are open, so the unrealized
result sits beside it and equity states what the balance would settle at if
everything were closed now. All three come from one frame, which keeps them
describing the same instant.
*/
export const Balance = () => {
	return (
		<Component registerKey="equity">
			{({ ref, className }) => (
				<Flex.Row ref={ref} align="center" gap={6} className={cn(className)}>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Cash
						</span>
						<span
							data-paint="cash"
							data-paint-format=".2f"
							className="font-mono text-[12px] font-medium text-(--f1)"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Unrealized
						</span>
						<span
							data-paint="unrealized"
							data-paint-format=".2f"
							className="font-mono text-[12px] font-medium text-(--f2)"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Equity
						</span>
						<span
							data-paint="equity"
							data-paint-format=".2f"
							className="font-mono text-[12px] font-semibold text-(--acc)"
						/>
					</Flex.Column>
				</Flex.Row>
			)}
		</Component>
	);
};
