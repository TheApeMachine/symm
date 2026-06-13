import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
} from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { balanceStore } from "#/collections/balance";
import { Page } from "#/components/layout/page";
import { PositionsPanel } from "#/components/panels/positions";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Popover, PopoverPopup, PopoverTrigger } from "#/components/ui/popover";
import { ToastProvider } from "#/components/ui/toast";
import { cn } from "#/lib/utils";
import { ThemeProvider } from "#/providers/theme";
import { WsFeed } from "#/providers/websocket";
import appCss from "../styles.css?url";

const ConnectionBadge = () => {
	const appState = useSelector(appStore, (state) => state);

	return (
		<Badge variant="outline" className="rounded-full">
			<span
				aria-hidden="true"
				className={cn(
					"size-2 rounded-full",
					appState.online ? "bg-emerald-500" : "bg-red-500",
				)}
			/>
			{appState.online ? "Online" : "Offline"}
		</Badge>
	);
};

const formatQuoteAmount = (symbol: string, value: number) => {
	if (symbol.length === 1) {
		return `${symbol}${value.toFixed(2)}`;
	}

	return `${value.toFixed(2)} ${symbol}`;
};

const formatSignedQuoteAmount = (symbol: string, value: number) => {
	const sign = value >= 0 ? "+" : "-";
	const absolute = Math.abs(value);

	return `${sign}${formatQuoteAmount(symbol, absolute)}`;
};

const PageHeader = () => {
	const appState = useSelector(appStore, (state) => state);
	const balanceState = useSelector(balanceStore, (state) => state);
	const { updateShowPositions } = appStore.actions;
	const pricingLabel =
		balanceState.openPositions > 0 &&
		balanceState.pricedPositions < balanceState.openPositions
			? ` · ${balanceState.pricedPositions}/${balanceState.openPositions} priced`
			: "";

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
									onClick={() => updateShowPositions(!appState.showPositions)}
								/>
							}
						>
							<div className="flex flex-col gap-0.5">
								<h3 className="flex flex-wrap items-baseline gap-x-1.5">
									<span>{balanceState.balanceLabel}</span>
									{balanceState.pricedPositions > 0 ? (
										<>
											<span className="text-base font-normal text-muted-foreground">
												· Equity{" "}
												{formatQuoteAmount(
													balanceState.symbol,
													balanceState.liquidationBalance,
												)}
											</span>
											<span
												className={cn(
													"text-base font-normal",
													balanceState.inProfit
														? "text-emerald-400"
														: "text-red-400",
												)}
											>
												· Open P&amp;L{" "}
												{formatSignedQuoteAmount(
													balanceState.symbol,
													balanceState.exitBalance,
												)}
											</span>
										</>
									) : null}
									{balanceState.liquidationComplete &&
									balanceState.pricedPositions > 0 ? (
										balanceState.inProfit ? (
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
									{balanceState.openPositions} open position
									{balanceState.openPositions === 1 ? "" : "s"}
									{pricingLabel}
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
								{appState.online ? appState.playbookEvaluations : "…"}
							</h3>
							<p className="font-normal text-muted-foreground">
								playbook evaluations
							</p>
						</Flex.Row>
						<p className="text-xs font-normal text-muted-foreground">
							{appState.online
								? `${appState.storyTicks} story ticks`
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
	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				<HeadContent />
				<script src="/theme-init.js" />
			</head>
			<body className="flex h-full min-h-svh flex-col" suppressHydrationWarning>
				<ThemeProvider>
					<WsFeed />
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
