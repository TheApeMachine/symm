import { motion } from "motion/react";
import type { ReactNode } from "react";
import { spring } from "#/components/ui/dynamic-island/animation";
import type {
	ShapeConfig,
	ShapeKey,
} from "#/components/ui/dynamic-island/types";
import { Typography } from "#/components/ui/typography";
import { cn } from "@/lib/utils";

const Cta = ({ label, className }: { label: string; className?: string }) => {
	return (
		<motion.div
			layoutId="cta"
			transition={spring}
			className={cn(
				"grid h-9 place-items-center rounded-full bg-brand px-4 text-sm font-semibold text-brand-foreground shadow-xs whitespace-nowrap",
				className,
			)}
		>
			<motion.span layout="position">{label}</motion.span>
		</motion.div>
	);
};

const Brand = ({ className }: { className?: string }) => {
	return (
		<motion.div
			layoutId="brand"
			transition={spring}
			aria-label="Island"
			className={cn(
				"grid h-6 w-[110px] place-items-center text-sm font-extrabold whitespace-nowrap",
				className,
			)}
		>
			<motion.span layout="position" aria-hidden>
				◆ ISLAND
			</motion.span>
		</motion.div>
	);
};

const Field = ({ label }: { label: string }) => (
	<div className="mb-2.5">
		<Typography.Small variant="muted" className="mb-1 block">
			{label}
		</Typography.Small>
		<div className="h-[34px] rounded-lg border border-border/80 bg-muted/40" />
	</div>
);

const Btn = ({ children }: { children: ReactNode }) => (
	<div className="grid h-9 place-items-center rounded-full bg-secondary px-[18px] text-sm font-semibold text-secondary-foreground">
		{children}
	</div>
);

export const SHAPES: Record<ShapeKey, ShapeConfig> = {
	pill: {
		label: "Pill",
		shellClassName: "h-9 w-[116px] rounded-[18px]",
		regions: {
			m: (
				<output
					className="grid h-full place-items-center text-xs text-muted-foreground"
					aria-live="polite"
				>
					<Typography.Small variant="muted">● recording</Typography.Small>
				</output>
			),
		},
	},

	button: {
		label: "Button",
		shellClassName: "h-12 w-[152px] rounded-3xl p-1.5",
		regions: {
			m: (
				<div className="grid h-full place-items-center">
					<Cta label="Sign in" className="h-9 w-32" />
				</div>
			),
		},
	},

	toast: {
		label: "Toast",
		shellClassName: "h-16 w-[300px] gap-3 rounded-2xl p-3",
		regions: {
			n: (
				<div
					className="grid size-10 place-items-center rounded-xl bg-brand/20 text-lg"
					aria-hidden
				>
					✦
				</div>
			),
			m: (
				<output className="grid h-full content-center" aria-live="polite">
					<Typography.Paragraph className="text-sm font-semibold">
						Saved to library
					</Typography.Paragraph>
					<Typography.Small variant="muted">Undo · just now</Typography.Small>
				</output>
			),
		},
	},

	form: {
		label: "Form",
		shellClassName: "h-[304px] w-[300px] gap-4 rounded-[22px] p-[22px]",
		regions: {
			h: (
				<Typography.PageTitle className="text-lg">Sign in</Typography.PageTitle>
			),
			m: (
				<div>
					<Field label="Email" />
					<Field label="Password" />
				</div>
			),
			f: <Cta label="Sign in" className="h-10 w-full" />,
		},
	},

	card: {
		label: "Card",
		shellClassName: "h-80 w-[280px] rounded-[22px]",
		regions: {
			h: (
				<div
					className="h-[130px] bg-linear-to-br from-brand/70 to-brand"
					aria-hidden
				/>
			),
			m: (
				<div className="px-5 py-4">
					<Typography.Paragraph className="text-base font-bold">
						Northern Lights
					</Typography.Paragraph>
					<Typography.Small
						variant="muted"
						className="mt-1.5 block leading-normal"
					>
						One surface, folded into a media card. Same grid, different regions
						lit.
					</Typography.Small>
				</div>
			),
			f: (
				<div className="flex gap-2.5 px-5 pb-[18px]">
					<Btn>Open</Btn>
					<Btn>Share</Btn>
				</div>
			),
		},
	},

	nav: {
		label: "Nav bar",
		shellClassName: "h-14 w-[560px] rounded-2xl px-[18px]",
		regions: {
			n: (
				<div className="grid h-full place-items-center pr-6">
					<Brand />
				</div>
			),
			m: (
				<nav
					className="flex h-full items-center gap-[22px] text-sm text-muted-foreground"
					aria-label="Primary"
				>
					<Typography.Span>Home</Typography.Span>
					<Typography.Span>Work</Typography.Span>
					<Typography.Span>About</Typography.Span>
				</nav>
			),
			a: (
				<div className="grid h-full place-items-center pl-6">
					<Cta label="Get started" className="h-9 w-[120px]" />
				</div>
			),
		},
	},

	page: {
		label: "Full page",
		shellClassName: "h-[440px] w-[620px] rounded-[22px]",
		regions: {
			h: (
				<header className="flex h-14 items-center justify-between border-b border-border/80 px-[22px]">
					<Brand />
					<Cta label="Get started" className="h-9 w-[120px]" />
				</header>
			),
			n: (
				<nav
					className="grid h-full w-[150px] content-start gap-3.5 border-r border-border/80 p-4 text-sm text-muted-foreground"
					aria-label="Sidebar"
				>
					<Typography.Span>Overview</Typography.Span>
					<Typography.Span>Regions</Typography.Span>
					<Typography.Span>Morphs</Typography.Span>
					<Typography.Span>Identity</Typography.Span>
				</nav>
			),
			m: (
				<div className="p-[26px]">
					<Typography.PageTitle className="text-[22px] font-extrabold">
						It's the same div.
					</Typography.PageTitle>
					<Typography.Small
						variant="muted"
						className="mt-3 block max-w-[280px] leading-relaxed"
					>
						One component, three Motion props. The button and logo carry a
						layoutId, so they travel between regions. Everything else enters and
						exits in the direction its track opens and closes — and you can
						interrupt any morph mid-flight.
					</Typography.Small>
				</div>
			),
			a: (
				<aside className="h-full w-[130px] border-l border-border/80 p-[18px]">
					<Typography.Small variant="muted">aside region</Typography.Small>
				</aside>
			),
			f: (
				<footer className="grid h-11 place-items-center border-t border-border/80">
					<Typography.Small variant="muted">footer region</Typography.Small>
				</footer>
			),
		},
	},
};

export const SHAPE_ORDER: ShapeKey[] = [
	"pill",
	"button",
	"toast",
	"form",
	"card",
	"nav",
	"page",
];
