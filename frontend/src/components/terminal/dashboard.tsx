import type { ReactNode } from "react";
import {
  TerminalFluidChart,
  TerminalPredictionChart,
} from "#/components/terminal/charts";
import type { TerminalModel } from "#/components/terminal/model";
import { TerminalSection } from "#/components/terminal/panels";
import {
  AuditRows,
  DecisionRows,
  KernelList,
  PositionRows,
} from "#/components/terminal/rows";
import { KernelInspector } from "#/components/terminal/widgets";
import { cn } from "#/lib/utils";

const DashboardPulse = ({ model }: { model: TerminalModel }) => (
  <div className="flex h-8 shrink-0 items-center gap-4 border-[var(--line)] border-b bg-[var(--sunken)] px-4 font-mono text-[11px] text-[var(--f4)]">
    <span className="font-semibold text-[var(--f1)]">
      {model.engine.sequence}
    </span>
    <span>
      phase{" "}
      <span className="font-semibold text-[var(--acc)]">
        {model.engine.phase}
      </span>
    </span>
    <span>meas {model.engine.measurements.toLocaleString()}</span>
    <span>cand {model.engine.candidates}</span>
    <span>open {model.engine.open}</span>
    <span>quotes {model.engine.signalsText}</span>
    <span>fluid {model.engine.fluidText}</span>
  </div>
);

const DashboardCanvasPanel = ({
  title,
  meta,
  children,
  className,
}: {
  title: string;
  meta: string;
  children: ReactNode;
  className: string;
}) => (
  <div
    className={cn("relative min-h-0 overflow-hidden bg-[#0a0907]", className)}
  >
    <div className="absolute inset-0">{children}</div>
    <div className="pointer-events-none absolute inset-0 opacity-50 [background-image:repeating-linear-gradient(0deg,rgba(0,0,0,0.18)_0px,rgba(0,0,0,0.18)_1px,transparent_1px,transparent_3px)] mix-blend-multiply" />
    <div className="pointer-events-none absolute top-3 left-3">
      <div className="font-semibold text-[10px] text-[var(--f2)] uppercase tracking-[0.13em]">
        {title}
      </div>
      <div className="mt-0.5 font-mono text-[9.5px] text-[var(--f4)]">
        {meta}
      </div>
    </div>
  </div>
);

export const DashboardSurface = ({
  model,
  selectedSource,
  inspectorSource,
  onInspect,
  onCloseInspect,
  onOpenInsight,
}: {
  model: TerminalModel;
  selectedSource: string;
  inspectorSource: string | null;
  onInspect: (source: string) => void;
  onCloseInspect: () => void;
  onOpenInsight: () => void;
}) => (
  <div className="flex h-full min-w-[1120px] flex-col">
    <DashboardPulse model={model} />
    <div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
      <KernelInspector
        kernel={
          model.kernels.find((kernel) => kernel.source === inspectorSource) ??
          null
        }
        onClose={onCloseInspect}
        onInsight={onOpenInsight}
      />
      <TerminalSection
        title="Signal kernels"
        meta={`${model.health.healthy}/${model.health.total} ok`}
        className="h-full min-h-0 rounded-none border-y-0 border-l-0"
      >
        <KernelList
          kernels={model.kernels}
          selectedSource={selectedSource}
          onSelect={onInspect}
        />
      </TerminalSection>

      <div className="flex min-h-0 flex-col border-[var(--line)] border-x bg-[var(--sunken)]">
        <DashboardCanvasPanel
          title="Fluid density field"
          meta="manifold rho · fluid carriers"
          className="flex-[1.45]"
        >
          <TerminalFluidChart />
        </DashboardCanvasPanel>
        <DashboardCanvasPanel
          title="Predictive coding"
          meta="backend prediction frames"
          className="flex-1 border-[var(--line)] border-t"
        >
          <TerminalPredictionChart />
        </DashboardCanvasPanel>
      </div>

      <div className="flex min-h-0 flex-col bg-[#17140f]">
        <TerminalSection
          title="Decisions"
          meta={model.engine.phase}
          className="min-h-0 flex-[1.15] rounded-none border-x-0 border-t-0"
        >
          <DecisionRows decisions={model.decisions} />
        </TerminalSection>
        <TerminalSection
          title="Open positions"
          meta={model.totalPnlText}
          className={cn(
            "min-h-0 flex-1 rounded-none border-x-0",
            model.totalPnlPositive ? "text-emerald-300" : "text-rose-300",
          )}
        >
          <PositionRows positions={model.positions} />
        </TerminalSection>
        <TerminalSection
          title="Audit trail"
          className="min-h-0 flex-1 rounded-none border-x-0 border-b-0"
        >
          <AuditRows rows={model.auditRows} />
        </TerminalSection>
      </div>
    </div>
  </div>
);
