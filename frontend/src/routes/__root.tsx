import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
} from "@tanstack/react-router";
import { WsFeed } from "#/providers/websocket";
import appCss from "../styles.css?url";

const RootDocument = ({ children }: { children: React.ReactNode }) => {
	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				<HeadContent />
			</head>
			<body className="flex h-full min-h-svh flex-col" suppressHydrationWarning>
				<ClientOnly fallback={null}>
					<WsFeed />
					{children}
				</ClientOnly>
				<Scripts />
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
		links: [
			{ rel: "stylesheet", href: appCss },
			{ rel: "preconnect", href: "https://fonts.googleapis.com" },
			{
				rel: "preconnect",
				href: "https://fonts.gstatic.com",
				crossOrigin: "anonymous",
			},
			{
				rel: "stylesheet",
				href: "https://fonts.googleapis.com/css2?family=Inter+Tight:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&family=Source+Serif+4:opsz,wght@8..60,400;8..60,600&display=swap",
			},
		],
	}),
	shellComponent: RootDocument,
});
