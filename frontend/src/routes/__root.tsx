import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
} from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Page } from "#/components/layout/page";
import { PositionsPanel } from "#/components/panels/positions";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Popover, PopoverPopup, PopoverTrigger } from "#/components/ui/popover";
import { ToastProvider } from "#/components/ui/toast";
import { cn, releaseSciChartWasm } from "#/lib/utils";
import { ThemeProvider } from "#/providers/theme";
import { useWsStatus, WsStatusProvider } from "#/providers/ws-status";
import appCss from "../styles.css?url";

const ConnectionBadge = () => {
	const { online } = useWsStatus();

	return (
		<Badge variant="outline" className="rounded-full">
			<span
				aria-hidden="true"
				className={cn(
					"size-2 rounded-full",
					online ? "bg-emerald-500" : "bg-red-500",
				)}
			/>
			{online ? "Online" : "Offline"}
		</Badge>
	);
};

const currencySymbol = (currency: string | null | undefined): string => {
	if (!currency) {
		return "";
	}

	const normalized = currency.toUpperCase();

	switch (normalized) {
		case "USD":
			return "$";
		case "EUR":
			return "€";
		default:
			return normalized;
	}
};

const PageHeader = () => {
	const {
		balance,
		currency,
		openPositions,
		exitBalance,
		capitalBase,
		online,
		storyTicks,
		playbookEvaluations,
	} = useWsStatus();
	const [showPositions, setShowPositions] = useState(false);
	const inProfit =
		exitBalance !== null && capitalBase > 0 && exitBalance >= capitalBase;
	const symbol = currencySymbol(currency);
	const balanceLabel =
		symbol.length === 1
			? `${symbol}${balance.toFixed(2)}`
			: `${symbol} ${balance.toFixed(2)}`;

	return (
		<Page.Header>
			<div className="flex flex-wrap items-center gap-2">
				<div className="relative">
					<Popover>
						<PopoverTrigger
							render={
								<Button
									className="h-auto! gap-4 px-4 py-3 text-left"
									variant="outline"
									onClick={() => setShowPositions((open) => !open)}
								/>
							}
						>
							<div className="flex flex-col gap-0.5">
								<h3 className="flex flex-wrap items-baseline gap-x-1.5">
									<span>{balanceLabel}</span>
									{openPositions > 0 && exitBalance !== null ? (
										<span
											className={cn(
												"text-base font-normal",
												inProfit ? "text-emerald-400" : "text-red-400",
											)}
										>
											(
											{symbol.length === 1
												? `${symbol}${exitBalance.toFixed(2)}`
												: `${symbol} ${exitBalance.toFixed(2)}`}
											)
										</span>
									) : null}
									{openPositions > 0 && exitBalance !== null ? (
										inProfit ? (
											<img
												src="/lambo.png"
												alt="Lambo"
												className="size-4 drop-shadow-[0_0_8px_rgba(239,68,68,1)]"
											/>
										) : (
											<span>💀</span>
										)
									) : null}
								</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									{openPositions} open position{openPositions === 1 ? "" : "s"}
								</p>
							</div>
						</PopoverTrigger>
						<PopoverPopup className="w-80">
							<PositionsPanel />
						</PopoverPopup>
					</Popover>
				</div>

				<Button className="h-auto! px-4 py-3 text-left" variant="outline">
					<div className="flex flex-col gap-0.5">
						<Flex.Row gap={1} align="center">
							<h3 className="tabular-nums">
								{online ? playbookEvaluations.toLocaleString() : "…"}
							</h3>
							<p className="font-normal text-muted-foreground">
								playbook evaluations
							</p>
						</Flex.Row>
						<p className="text-xs font-normal text-muted-foreground">
							{online
								? `${storyTicks.toLocaleString()} story ticks`
								: "story ticks"}
						</p>
					</div>
				</Button>
			</div>
			<ConnectionBadge />
		</Page.Header>
	);
};

const RootDocument = ({ children }: { children: React.ReactNode }) => {
	useEffect(() => {
		return () => {
			releaseSciChartWasm();
		};
	}, []);

	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				<HeadContent />
				<script src="/theme-init.js" />
			</head>
			<body className="flex h-full min-h-svh flex-col" suppressHydrationWarning>
				<ThemeProvider>
					<WsStatusProvider>
						<ToastProvider>
							<Page>
								<PageHeader />
								<Page.Nav />
								<Page.Main>
									<Page.MainBody>
										<ClientOnly fallback={null}>{children}</ClientOnly>
									</Page.MainBody>
								</Page.Main>
								<Page.Aside>{/* reserved for layout */}</Page.Aside>
								<Page.Footer />
							</Page>
						</ToastProvider>
						<Scripts />
					</WsStatusProvider>
				</ThemeProvider>
			</body>
		</html>
	);
};

export const Route = createRootRoute({
	head: () => ({
		meta: [
			{ charSet: "utf-8" },
			{ name: "viewport", content: "width=device-width, initial-scale=1" },
			{ title: "symm" },
		],
		links: [{ rel: "stylesheet", href: appCss }],
	}),
	shellComponent: RootDocument,
});
