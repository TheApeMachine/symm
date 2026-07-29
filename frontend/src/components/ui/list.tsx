import { Flex } from "@/components/ui/flex";
import { cn } from "@/lib/utils";

interface ListProps {
	className?: string;
	children: React.ReactNode;
}

export const List = ({ className, children }: ListProps) => {
	return (
		<Flex.Column className={cn("gap-1", className)}>{children}</Flex.Column>
	);
};

interface ListItemProps {
	className?: string;
	children: React.ReactNode;
}

List.Item = ({ className, children }: ListItemProps) => {
	return (
		<Flex.Row
			gap={2}
			align="center"
			className={cn(
				"rounded-[3px] px-2 py-1 text-[11px] font-medium text-(--f1) transition-colors hover:bg-(--sunken)",
				className,
			)}
		>
			{children}
		</Flex.Row>
	);
};
