import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

export const Balance = () => {
	return (
		<Component registerKey="balances">
			{({ ref, className }) => (
				<Flex.Row ref={ref} align="center" gap={6} className={cn(className)}>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Balance
						</span>
						<span
							data-paint="USD.balance"
							className="font-mono text-[12px] font-medium text-(--f1)"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Available
						</span>
						<span
							data-paint="USD.available"
							className="font-mono text-[12px] font-medium text-(--f3)"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Reserved
						</span>
						<span
							data-paint="USD.reserved"
							className="relative font-mono text-[12px] font-semibold"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Asset
						</span>
						<span
							data-paint="USD.asset"
							className="font-mono text-[12px] font-semibold text-(--acc)"
						/>
					</Flex.Column>
				</Flex.Row>
			)}
		</Component>
	);
};
