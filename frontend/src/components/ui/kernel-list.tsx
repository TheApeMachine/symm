import {
	KernelList as BaseKernelList,
	type KernelListProps,
} from "#/components/terminal/kernel-list";

export type { KernelListProps };

export const KernelList = (props: KernelListProps) => (
	<BaseKernelList {...props} />
);
