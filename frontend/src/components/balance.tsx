import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

export const Balance = () => {
	return (
		<Component registerKey="balances">
			{({ ref, className }) => (
				<Flex.Row
					ref={ref}
					align="center"
					gap={6}
					className={cn(className)}
					data-scope="asset"
					data-filter="USD"
				>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Cash
						</span>
						<span
							data-paint="USD"
							className="font-mono text-[12px] font-medium text-(--f1)"
						/>
					</Flex.Column>
					<Flex.Column className="items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Asset
						</span>
						<span className="font-mono text-[12px] font-semibold text-(--acc)">
							USD
						</span>
					</Flex.Column>
				</Flex.Row>
			)}
		</Component>
	);
};
