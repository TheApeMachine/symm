import { Flex } from "@/components/ui/flex";
import { cn } from "@/lib/utils";

interface ListProps {
	className?: string;
	children: React.ReactNode;
	ref?: React.Ref<HTMLDivElement>;
}

export const List = ({ className, children, ref }: ListProps) => {
	return (
		<Flex.Column ref={ref} className={cn("gap-1", className)}>
			{children}
		</Flex.Column>
	);
};

interface ListItemProps {
	className?: string;
	children: React.ReactNode;
	ref?: React.Ref<HTMLDivElement>;
	[key: string]: unknown;
}

List.Item = ({ className, children, ref, ...props }: ListItemProps) => {
	return (
		<Flex.Row
			ref={ref}
			gap={2}
			align="center"
			{...props}
			className={cn(
				"rounded-[3px] px-2 py-1 text-[11px] font-medium text-(--f1) transition-colors hover:bg-(--sunken)",
				className,
			)}
		>
			{children}
		</Flex.Row>
	);
};
