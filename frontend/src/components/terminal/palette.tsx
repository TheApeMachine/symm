import { ActivityIcon, LayoutDashboardIcon, ScanEyeIcon } from "lucide-react";
import type {
  TerminalModel,
  TerminalSurface,
} from "#/components/terminal/model";
import { toneClasses } from "#/components/terminal/tone";
import { cn } from "#/lib/utils";

const SURFACES: Array<{ id: TerminalSurface; label: string; hint: string }> = [
  { id: "dashboard", label: "Dashboard", hint: "Fluid field · live decisions" },
  { id: "signals", label: "Signal insight", hint: "Per-kernel forensics" },
  { id: "decisions", label: "Decision tree", hint: "Gate-by-gate trace" },
  { id: "xray", label: "Latent x-ray", hint: "State-space cross-section" },
  { id: "cortex", label: "Cognitive tree", hint: "Reasoning graph" },
  { id: "allocation", label: "Allocation", hint: "Capital & exposure" },
];

export const CommandPalette = ({
  open,
  query,
  model,
  activeSurface,
  activeIndex,
  onQuery,
  onClose,
  onRun,
}: {
  open: boolean;
  query: string;
  model: TerminalModel;
  activeSurface: TerminalSurface;
  activeIndex: number;
  onQuery: (query: string) => void;
  onClose: () => void;
  onRun: (surface: TerminalSurface, source?: string) => void;
}) => {
  if (!open) {
    return null;
  }

  const commands = [
    ...SURFACES.map((surface) => ({
      key: `surface:${surface.id}`,
      label: surface.label,
      hint: surface.hint,
      group: "Surface",
      surface: surface.id,
      source: undefined as string | undefined,
    })),
    ...model.kernels.map((kernel) => ({
      key: `kernel:${kernel.source}`,
      label: `Inspect · ${kernel.name}`,
      hint: kernel.category,
      group: "Kernel",
      surface: "dashboard" as TerminalSurface,
      source: kernel.source,
    })),
  ].filter((command) =>
    `${command.label} ${command.hint}`
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const selectedIndex = commands.length > 0 ? activeIndex % commands.length : 0;

  return (
    <div
      className="absolute inset-0 z-40 flex items-start justify-center bg-black/65 pt-24 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="flex max-h-[60vh] w-[560px] max-w-[calc(100%-48px)] flex-col overflow-hidden rounded-lg border border-stone-700 bg-[#17140f] shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-stone-800 border-b px-4 py-3">
          <ScanEyeIcon className="size-4 text-stone-600" />
          <input
            autoFocus
            value={query}
            onChange={(event) => onQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Enter") {
                return;
              }

              const command = commands[selectedIndex];

              if (command === undefined) {
                return;
              }

              event.preventDefault();
              onRun(command.surface, command.source);
            }}
            placeholder="Jump to a surface or kernel..."
            className="min-w-0 flex-1 bg-transparent text-stone-100 outline-none"
          />
          <span className="font-mono text-[10px] text-stone-600">
            {commands.length} commands
          </span>
        </div>
        <div className="min-h-0 overflow-auto p-1.5">
          {commands.map((command, index) => {
            const active = index === selectedIndex;

            return (
              <button
                type="button"
                key={command.key}
                onClick={() => onRun(command.surface, command.source)}
                className={cn(
                  "flex w-full items-center gap-3 rounded-sm border-l-2 px-3 py-2 text-left",
                  active
                    ? "border-l-amber-300 bg-stone-800"
                    : "border-l-transparent",
                )}
              >
                <span className="flex size-6 items-center justify-center rounded-sm border border-stone-800 bg-black/25 text-amber-300">
                  {command.group === "Surface" ? (
                    <LayoutDashboardIcon className="size-3.5" />
                  ) : (
                    <ActivityIcon className="size-3.5" />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-stone-100 text-sm">
                    {command.label}
                  </span>
                  <span className="block truncate font-mono text-[10px] text-stone-600">
                    {command.hint}
                  </span>
                </span>
                <span
                  className={cn(
                    "rounded-sm border px-1.5 py-0.5 font-semibold text-[9px] uppercase",
                    command.surface === activeSurface
                      ? toneClasses("warn")
                      : toneClasses("info"),
                  )}
                >
                  {command.group}
                </span>
              </button>
            );
          })}
        </div>
        <div className="flex gap-4 border-stone-800 border-t bg-black/25 px-4 py-2 font-mono text-[10px] text-stone-600">
          <span>up/down navigate</span>
          <span>enter open</span>
          <span>esc close</span>
          <span className="ml-auto">cmd+k toggle</span>
        </div>
      </div>
    </div>
  );
};
