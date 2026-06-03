import { Link, useLocation, useRouterState } from "@tanstack/react-router";
import { Menu as MenuIcon } from "lucide-react";
import type React from "react";
import { useState } from "react";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/ui/breadcrumb";
import { Button } from "#/components/ui/button";
import {
	Sheet,
	SheetClose,
	SheetDescription,
	SheetFooter,
	SheetHeader,
	SheetPanel,
	SheetPopup,
	SheetTitle,
	SheetTrigger,
} from "#/components/ui/sheet";
import { cn } from "#/lib/utils";
import { Flex } from "../ui/flex";
import { Navigation } from "./navigation";

const UUID_PATTERN =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const humanize = (segment: string) => {
	const decoded = decodeURIComponent(segment);

	if (UUID_PATTERN.test(decoded)) {
		return `${decoded.slice(0, 8)}…${decoded.slice(-4)}`;
	}

	return decoded
		.replace(/[-_]+/g, " ")
		.replace(/\b\w/g, (character) => character.toUpperCase());
};

/*
PageContentWidth selects how routed main content is sized in the shell.
full: span the main grid column.
contained: centered column with a max width (document-style layouts).
*/
export type PageContentWidth = "full" | "contained";

type PageContentWidthStatic = {
	pageContentWidth?: PageContentWidth;
};

/*
resolvePageContentWidth walks active matches from leaf to root; the first
explicit pageContentWidth wins so child routes can override layout parents.
*/
export function resolvePageContentWidth(
	matches: ReadonlyArray<{ staticData?: unknown }>,
): PageContentWidth {
	for (let index = matches.length - 1; index >= 0; index--) {
		const data = matches[index]?.staticData as
			| PageContentWidthStatic
			| undefined;
		const width = data?.pageContentWidth;

		if (width === "contained" || width === "full") {
			return width;
		}
	}

	return "full";
}

const useRouteCrumbs = () => {
	const { pathname } = useLocation();
	const segments = pathname.split("/").filter(Boolean);

	return segments.map((segment, index) => {
		const href = `/${segments.slice(0, index + 1).join("/")}`;

		return {
			href,
			label: humanize(segment),
		};
	});
};

const RouteCrumb = ({
	href,
	label,
	isLast,
}: {
	href: string;
	label: string;
	isLast: boolean;
}) => {
	return (
		<>
			<BreadcrumbSeparator />
			<BreadcrumbItem>
				{isLast ? (
					<BreadcrumbPage>{label}</BreadcrumbPage>
				) : (
					<BreadcrumbLink href={href}>{label}</BreadcrumbLink>
				)}
			</BreadcrumbItem>
		</>
	);
};

export const Page = ({ children }: { children?: React.ReactNode }) => {
	return <>{children ?? null}</>;
};

Page.Header = ({ children }: { children?: React.ReactNode }) => {
	return <PageHeaderBody>{children}</PageHeaderBody>;
};

const PageHeaderBody = ({ children }: { children?: React.ReactNode }) => {
	const [navOpen, setNavOpen] = useState(false);

	return (
		<header className="grid-area-header flex shrink-0 flex-wrap items-center justify-between gap-4 p-4">
			<Breadcrumb className="min-w-0 flex-1">
				<BreadcrumbList>
					<BreadcrumbItem>
						<Sheet open={navOpen} onOpenChange={setNavOpen}>
							<SheetTrigger render={<Button variant="outline" />}>
								<MenuIcon />
							</SheetTrigger>
							<SheetPopup variant="inset" side="left">
								<SheetHeader>
									<SheetTitle>Symm</SheetTitle>
									<SheetDescription>
										...Like somebody 'bout to pay ya
									</SheetDescription>
								</SheetHeader>
								<SheetPanel className="grid gap-4">
									<Navigation onNavigate={() => setNavOpen(false)} />
								</SheetPanel>
								<SheetFooter>
									<SheetClose render={<Button variant="ghost" />}>
										Cancel
									</SheetClose>
									<Button type="submit">Save</Button>
								</SheetFooter>
							</SheetPopup>
						</Sheet>
					</BreadcrumbItem>
					<BreadcrumbItem>
						<BreadcrumbLink render={<Link to={"/"} />}>Home</BreadcrumbLink>
					</BreadcrumbItem>
					{useRouteCrumbs().map((crumb, index, all) => (
						<RouteCrumb
							key={crumb.href}
							href={crumb.href}
							isLast={index === all.length - 1}
							label={crumb.label}
						/>
					))}
				</BreadcrumbList>
			</Breadcrumb>

			{children ?? null}
		</header>
	);
};

Page.Nav = ({ children }: { children?: React.ReactNode[] }) => {
	return <nav className="grid-area-nav shrink-0">{children ?? null}</nav>;
};

Page.Main = ({ children }: { children?: React.ReactNode }) => {
	return (
		<main className="flex h-full min-h-0 flex-1 flex-col">
			{children ?? null}
		</main>
	);
};

/*
Page.MainBody wraps routed outlet content and honors each route staticData
pageContentWidth. Use it once inside Page.Main in the root shell.
*/
Page.MainBody = ({ children }: { children?: React.ReactNode }) => {
	const contentWidth = useRouterState({
		select: (state) => resolvePageContentWidth(state.matches),
	});

	if (contentWidth === "contained") {
		return (
			<div className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col">
				<div
					className={cn(
						"mx-auto flex h-full min-h-0 w-full max-w-6xl flex-1 flex-col",
						"px-4 sm:px-6",
					)}
				>
					{children ?? null}
				</div>
			</div>
		);
	}

	return (
		<Flex.Column padding={2} fullWidth fullHeight>
			{children ?? null}
		</Flex.Column>
	);
};

Page.Section = ({ children }: { children?: React.ReactNode }) => {
	return <section>{children ?? null}</section>;
};

Page.Aside = ({ children }: { children?: React.ReactNode }) => {
	return <aside className="shrink-0">{children ?? null}</aside>;
};

Page.Footer = ({ children: _children }: { children?: React.ReactNode }) => {
	return <footer className="shrink-0"></footer>;
};
