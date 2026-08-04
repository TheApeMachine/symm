import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";
import { Balance } from "@/components/balance";
import { Count } from "@/components/count";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Key } from "@/components/ui/key";
import { Toolbar } from "@/components/ui/toolbar";
import { Component, Flex, Typography } from "../ui";

const SymmLogo = () => (
	<svg
		width="22"
		height="22"
		viewBox="0 0 22 22"
		fill="none"
		className="block"
		aria-hidden="true"
	>
		<title>SYMM</title>
		<circle cx="11" cy="11" r="8.5" stroke="var(--acc)" strokeWidth="1.3" />
		<circle cx="11" cy="11" r="3.4" stroke="var(--acc)" strokeWidth="1.3" />
		<path
			d="M11 0.5V5M11 17V21.5M0.5 11H5M17 11H21.5"
			stroke="var(--acc)"
			strokeWidth="1.3"
		/>
	</svg>
);

export const TerminalTopBar = () => {
	const online = useSelector(appStore, (state) => state.online);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { openPalette, openSymbolPalette } = terminalStore.actions;

	return (
		<Toolbar size="lg" className="relative z-5">
			<Toolbar.Group>
				<SymmLogo />
				<span className="font-semibold text-[14px] text-(--f1) tracking-[0.22em]">
					SYMM
				</span>
			</Toolbar.Group>

			<Badge
				label={online ? "live" : "offline"}
				variant={online ? "success" : "error"}
				dot
				pulse={online}
			/>
			<Count />

			<Toolbar.Spacer />

			<Toolbar.Group className="gap-6">
				<Button
					variant="bare"
					onClick={openSymbolPalette}
					data-symbol={focusSymbol}
					title="Search focused symbol"
				>
					<Badge
						label={focusSymbol}
						variant="warning"
						size="m"
						className="font-mono"
					/>
				</Button>

				<Button variant="outline" size="m" onClick={openPalette}>
					<Icon name="search" size="s" />
					<span className="text-[11px]">Jump to</span>
					<Key>k</Key>
				</Button>

				<Balance />
			</Toolbar.Group>
			<Toolbar.Group>
				<Component registerKey="strategy">
					{({ ref, className }) => (
						<Flex.Row
							ref={ref}
							align="center"
							gap={6}
							className={cn(className)}
						>
							<Flex.Column className="items-end gap-px">
								<Typography.Label size="s" tone="f4" weight="normal">
									Tick
								</Typography.Label>
								<Typography.Mono size="lg" tone="f1" data-paint="tick" />
							</Flex.Column>
						</Flex.Row>
					)}
				</Component>
			</Toolbar.Group>
		</Toolbar>
	);
};
