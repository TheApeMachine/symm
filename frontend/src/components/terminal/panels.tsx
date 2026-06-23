import {
  ActivityIcon,
  BrainCircuitIcon,
  ChartNoAxesColumnIncreasingIcon,
  CircleDotIcon,
  GitBranchIcon,
  LayoutDashboardIcon,
  RadioIcon,
  ScanEyeIcon,
  SearchIcon,
  WavesIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type {
  TerminalModel,
  TerminalSurface,
} from "#/components/terminal/model";
import { toneClasses } from "#/components/terminal/tone";
import { Button } from "#/components/ui/button";
import { cn } from "#/lib/utils";

const SURFACE_ITEMS: Array<{
  key: TerminalSurface;
  label: string;
  icon: ReactNode;
}> = [
  { key: "dashboard", label: "Dashboard", icon: <LayoutDashboardIcon /> },
  { key: "signals", label: "Signal insight", icon: <ActivityIcon /> },
  { key: "decisions", label: "Decision tree", icon: <GitBranchIcon /> },
  { key: "xray", label: "Latent x-ray", icon: <ScanEyeIcon /> },
  { key: "cortex", label: "Cognitive tree", icon: <BrainCircuitIcon /> },
  {
    key: "allocation",
    label: "Allocation",
    icon: <ChartNoAxesColumnIncreasingIcon />,
  },
];

const TerminalChrome = ({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) => (
  <div
    className={cn(
      "rounded-md border border-stone-700/80 bg-stone-950/70 shadow-[0_20px_80px_-50px_rgba(0,0,0,0.85)]",
      className,
    )}
  >
    {children}
  </div>
);

export const TerminalSection = ({
  title,
  meta,
  children,
  className,
}: {
  title: string;
  meta?: ReactNode;
  children: ReactNode;
  className?: string;
}) => (
  <TerminalChrome className={className}>
    <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-md">
      <div className="flex h-9 shrink-0 items-center justify-between gap-3 border-stone-800 border-b px-3">
        <span className="font-semibold text-[10px] text-stone-400 uppercase tracking-[0.16em]">
          {title}
        </span>
        {meta ? (
          <span className="font-mono text-[10px] text-stone-500">{meta}</span>
        ) : null}
      </div>
      {children}
    </div>
  </TerminalChrome>
);

export const TerminalTopBar = ({
  model,
  onOpenPalette,
}: {
  model: TerminalModel;
  onOpenPalette: () => void;
}) => (
  <header className="flex h-[52px] shrink-0 items-center gap-4 border-stone-800 border-b bg-stone-950 px-4">
    <div className="flex items-center gap-3">
      <CircleDotIcon className="size-5 text-amber-300" />
      <span className="font-semibold text-stone-100 tracking-[0.22em]">
        SYMM
      </span>
    </div>
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-sm border px-2 py-1 font-semibold text-[10px] uppercase tracking-[0.12em]",
        model.online ? toneClasses("good") : toneClasses("bad"),
      )}
    >
      <span
        className={cn(
          "size-1.5 rounded-full",
          model.online ? "bg-emerald-300" : "bg-rose-300",
        )}
      />
      {model.online ? "live" : "offline"}
    </span>
    <span className="font-mono text-stone-500 text-xs">
      {model.wallet.openText}
    </span>
    <Button
      variant="ghost"
      size="sm"
      onClick={onOpenPalette}
      className="ml-auto hidden gap-2 rounded-sm border border-stone-800 bg-black/30 px-2 text-stone-400 hover:border-stone-700 hover:text-stone-100 md:inline-flex"
    >
      <SearchIcon className="size-3.5" />
      <span>Jump to</span>
      <span className="rounded-sm border border-stone-800 px-1 font-mono text-[10px] text-stone-600">
        ⌘K
      </span>
    </Button>
    <div className="hidden items-center gap-5 md:flex">
      <TopMetric label="Cash" value={model.wallet.cash} />
      <TopMetric label="Available" value={model.wallet.available} />
      <TopMetric label="Reserved" value={model.wallet.reserved} />
      <TopMetric label="Tick" value={model.wallet.tick} accent />
    </div>
  </header>
);

const TopMetric = ({
  label,
  value,
  accent = false,
}: {
  label: string;
  value: string;
  accent?: boolean;
}) => (
  <div className="flex flex-col items-end leading-tight">
    <span className="font-semibold text-[9px] text-stone-600 uppercase tracking-[0.14em]">
      {label}
    </span>
    <span
      className={cn(
        "font-mono text-xs",
        accent ? "text-amber-300" : "text-stone-200",
      )}
    >
      {value}
    </span>
  </div>
);

export const TerminalNav = ({
  active,
  model,
  onSelect,
}: {
  active: TerminalSurface;
  model: TerminalModel;
  onSelect: (surface: TerminalSurface) => void;
}) => (
  <nav className="flex w-54 shrink-0 flex-col border-stone-800 border-r bg-stone-950/95">
    <div className="px-3 pt-3 pb-2 font-semibold text-[10px] text-stone-600 uppercase tracking-[0.16em]">
      Surfaces
    </div>
    <div className="flex flex-col gap-1 px-2">
      {SURFACE_ITEMS.map((item) => (
        <Button
          key={item.key}
          variant="ghost"
          size="sm"
          onClick={() => onSelect(item.key)}
          className={cn(
            "justify-start rounded-sm border border-transparent px-2 text-stone-400",
            active === item.key &&
              "border-amber-400/35 bg-amber-400/10 text-amber-100",
          )}
        >
          <span className="size-4">{item.icon}</span>
          {item.label}
        </Button>
      ))}
    </div>
    <div className="px-3 pt-5 pb-2 font-semibold text-[10px] text-stone-600 uppercase tracking-[0.16em]">
      Engine
    </div>
    <div className="mx-2 rounded-md border border-stone-800 bg-black/30 p-3 font-mono text-[11px] text-stone-400">
      <MetricLine label="seq" value={model.engine.sequence} />
      <MetricLine label="phase" value={model.engine.phase} accent />
      <MetricLine label="cand" value={model.engine.candidates.toString()} />
      <MetricLine label="open" value={model.engine.open.toString()} />
      <ProgressLine
        label="signals"
        text={model.engine.signalsText}
        value={model.engine.signalsPercent}
      />
      <ProgressLine
        label="pump"
        text={model.engine.fluidText}
        value={model.engine.fluidPercent}
        accent
      />
    </div>
    <div className="mt-auto border-stone-800 border-t p-3 font-mono text-[10px] text-stone-600">
      <div>{model.clockText} UTC</div>
      <div>walk {model.walkSymbol || "none"}</div>
    </div>
  </nav>
);

const MetricLine = ({
  label,
  value,
  accent = false,
}: {
  label: string;
  value: string;
  accent?: boolean;
}) => (
  <div className="flex justify-between gap-3">
    <span className="text-stone-600">{label}</span>
    <span className={accent ? "text-amber-300" : "text-stone-300"}>
      {value}
    </span>
  </div>
);

const ProgressLine = ({
  label,
  text,
  value,
  accent = false,
}: {
  label: string;
  text: string;
  value: number;
  accent?: boolean;
}) => (
  <div className="mt-2">
    <div className="mb-1 flex justify-between text-stone-600">
      <span>{label}</span>
      <span>{text}</span>
    </div>
    <div className="h-1 overflow-hidden rounded-sm bg-stone-800">
      <div
        className={cn("h-full", accent ? "bg-amber-300" : "bg-cyan-300")}
        style={{ width: `${value}%` }}
      />
    </div>
  </div>
);

export const TerminalToolbar = () => (
  <div className="flex h-8 shrink-0 items-center gap-2 border-stone-800 border-b bg-black/20 px-3 font-mono text-[10px] text-stone-500">
    <RadioIcon className="size-3.5 text-cyan-300" />
    <span>websocket</span>
    <WavesIcon className="ml-3 size-3.5 text-amber-300" />
    <span>artifact stream</span>
    <SearchIcon className="ml-auto size-3.5 text-stone-600" />
  </div>
);
