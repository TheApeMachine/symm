import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
} from "@tanstack/react-router";
import { ChevronRightIcon } from "lucide-react";
import { Page } from "#/components/layout/page";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { ToastProvider } from "#/components/ui/toast";
import { cn } from "#/lib/utils";
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

const PageHeader = () => {
	const { balance } = useWsStatus();

	return (
		<Page.Header>
			<Button className="h-auto! gap-4 px-4 py-3 text-left" variant="outline">
				<div className="flex flex-col gap-0.5">
					<h3>TICK {0}</h3>
					<p className="whitespace-break-spaces font-normal text-muted-foreground">
						{0} measurements
					</p>
				</div>
			</Button>
			<Button className="h-auto! gap-4 px-4 py-3 text-left" variant="outline">
				<div className="flex flex-col gap-0.5">
					<h3>€{balance.toFixed(2)}</h3>
					<p className="whitespace-break-spaces font-normal text-muted-foreground">
						0 open positions
					</p>
				</div>
			</Button>
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
