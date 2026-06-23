import type { ReactNode } from "react";
import {
  TerminalCognitiveChart,
  TerminalFluidChart,
  TerminalManifoldChart,
  TerminalPositionChart,
  TerminalPredictionChart,
  TerminalResonanceChart,
  TerminalSignalHeatmap,
} from "#/components/terminal/charts";
import { Fact } from "#/components/terminal/fact";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import type {
  TerminalModel,
  TerminalSurface,
} from "#/components/terminal/model";
import { TerminalSection, TerminalToolbar } from "#/components/terminal/panels";
import {
  AuditRows,
  DecisionRows,
  KernelList,
  PositionRows,
} from "#/components/terminal/rows";
import {
  AllocationView,
  DecisionFunnel,
  DecisionTablePanel,
  KernelInspector,
  SignalDetail,
} from "#/components/terminal/widgets";
import { cn } from "#/lib/utils";

const ChartPanel = ({
  title,
  meta,
  children,
  className,
}: {
  title: string;
  meta?: string;
  children: ReactNode;
  className?: string;
}) => (
  <TerminalSection title={title} meta={meta} className={className}>
    <div className="relative min-h-0 flex-1 overflow-hidden bg-[#101722]">
      {children}
    </div>
  </TerminalSection>
);

export const SurfaceHeader = ({
  title,
  meta,
}: {
  title: string;
  meta: string;
}) => (
  <div className="flex h-10 shrink-0 items-center justify-between gap-4 border-stone-800 border-b bg-black/25 px-4">
    <h1 className="font-semibold text-sm text-stone-100">{title}</h1>
    <span className="font-mono text-[10px] text-stone-500">{meta}</span>
  </div>
);

const DashboardSurface = ({
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
  <div className="relative grid h-full min-w-[1120px] grid-cols-[282px_minmax(420px,1fr)_332px]">
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

    <div className="flex min-h-0 flex-col border-stone-800 border-x bg-[#0a0907]">
      <TerminalToolbar />
      <ChartPanel
        title="Fluid density field"
        meta="live artifact stream"
        className="min-h-0 flex-[1.35] rounded-none border-x-0 border-t-0"
      >
        <TerminalFluidChart />
      </ChartPanel>
      <ChartPanel
        title="Predictive coding"
        meta="backend prediction frames"
        className="min-h-0 flex-1 rounded-none border-x-0 border-b-0"
      >
        <TerminalPredictionChart />
      </ChartPanel>
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
);

const SignalSurface = ({
  model,
  selectedSource,
  onSelect,
}: {
  model: TerminalModel;
  selectedSource: string;
  onSelect: (source: string) => void;
}) => (
  <div className="grid h-full min-w-[1080px] grid-cols-[230px_minmax(420px,1fr)_320px]">
    <TerminalSection
      title="Kernels"
      className="h-full min-h-0 rounded-none border-y-0 border-l-0"
    >
      <KernelList
        kernels={model.kernels}
        selectedSource={selectedSource}
        onSelect={onSelect}
      />
    </TerminalSection>
    <div className="min-h-0 overflow-auto bg-[#0e0c0a]">
      <SignalDetail
        kernel={
          model.kernels.find((kernel) => kernel.source === selectedSource) ??
          null
        }
      />
      <div className="px-5 pb-5">
        <div className="mb-2 font-semibold text-[10px] text-stone-500 uppercase tracking-[0.13em]">
          Cross-section · confidence heatmap
        </div>
        <div className="h-56 overflow-hidden rounded border border-stone-800">
          <TerminalSignalHeatmap kind="confidence" />
        </div>
      </div>
    </div>
    <div className="min-h-0 space-y-3 overflow-auto border-stone-800 border-l bg-[#17140f] p-3.5">
      <HealthPanel model={model} />
      <RadarPanel model={model} />
      <div className="h-56 overflow-hidden rounded border border-stone-800">
        <TerminalSignalHeatmap kind="surprise" />
      </div>
    </div>
  </div>
);

const DecisionSurface = ({ model }: { model: TerminalModel }) => (
  <div className="grid h-full min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
    <div className="min-h-0 space-y-3 overflow-auto p-4">
      <DecisionFunnel model={model} />
      <div className="flex items-center gap-4 rounded border border-stone-800 bg-black/25 px-3 py-2 font-mono text-[11px]">
        <span className="text-stone-500">entry line</span>
        <span className="font-semibold text-amber-300">
          {model.engine.phase}
        </span>
        <span className="text-stone-600">·</span>
        <span className="text-stone-500">support gate backend trace</span>
        <span className="ml-auto text-stone-600">
          playbook {model.playbookBranches}
        </span>
      </div>
      <DecisionTablePanel model={model} />
    </div>
    <div className="min-h-0 space-y-3 overflow-auto border-stone-800 border-l bg-[#17140f] p-3.5">
      <HealthPanel model={model} />
      <TerminalSection
        title="Open positions"
        meta={model.totalPnlText}
        className="min-h-72"
      >
        <PositionRows positions={model.positions} />
      </TerminalSection>
    </div>
  </div>
);

const XraySurface = ({ model }: { model: TerminalModel }) => (
  <div className="grid h-full min-w-[1080px] grid-cols-[230px_minmax(480px,1fr)_320px]">
    <TerminalSection
      title="Symbols"
      className="h-full min-h-0 rounded-none border-y-0 border-l-0"
    >
      <div className="min-h-0 overflow-auto">
        {model.decisions.slice(0, 16).map((decision) => (
          <div
            key={decision.key}
            className="border-stone-800 border-b px-3 py-2 font-mono text-xs"
          >
            <div className="font-semibold text-stone-100">
              {decision.symbol}
            </div>
            <div className="text-[10px] text-stone-600">
              {decision.scoreText} · {decision.verdict}
            </div>
          </div>
        ))}
      </div>
    </TerminalSection>
    <div className="grid min-h-0 grid-rows-2">
      <ChartPanel
        title="Latent layers"
        className="min-h-0 rounded-none border-t-0 border-x-0"
      >
        <TerminalResonanceChart />
      </ChartPanel>
      <ChartPanel
        title="Hawkes / prediction"
        className="min-h-0 rounded-none border-b-0 border-x-0"
      >
        <TerminalPredictionChart />
      </ChartPanel>
    </div>
    <div className="grid min-h-0 grid-rows-2 border-stone-800 border-l bg-[#17140f]">
      <ChartPanel
        title="Manifold"
        className="min-h-0 rounded-none border-x-0 border-t-0"
      >
        <TerminalManifoldChart />
      </ChartPanel>
      <ChartPanel
        title="Positions"
        className="min-h-0 rounded-none border-x-0 border-b-0"
      >
        <TerminalPositionChart positions={model.positions} />
      </ChartPanel>
    </div>
  </div>
);

const CortexSurface = ({ model }: { model: TerminalModel }) => (
  <div className="grid h-full min-w-[1080px] grid-cols-[minmax(560px,1fr)_340px]">
    <ChartPanel
      title="Cognitive memory · DMT radix tree"
      className="min-h-0 rounded-none border-y-0 border-l-0"
    >
      <TerminalCognitiveChart reading={model.cognitive} />
    </ChartPanel>
    <TerminalSection
      title="Cognitive state"
      meta={`${model.cognitiveScopes.length} scopes`}
      className="h-full min-h-0 rounded-none border-y-0 border-r-0"
    >
      <div className="space-y-3 p-3 font-mono text-xs">
        <Fact label="scope" value={model.cognitive?.scope ?? "none"} />
        <Fact
          label="regime"
          value={model.cognitive?.regimePrefix || "waiting"}
        />
        <Fact
          label="winner"
          value={model.cognitive?.winnerClass || "pending"}
        />
        <Fact
          label="entropy"
          value={model.cognitive ? model.cognitive.entropyBits.toFixed(3) : "0"}
        />
        <Fact
          label="lookahead"
          value={
            model.cognitive ? model.cognitive.lookaheadScore.toFixed(3) : "0"
          }
        />
      </div>
    </TerminalSection>
  </div>
);

const AllocationSurface = ({ model }: { model: TerminalModel }) => (
  <div className="flex h-full min-w-[1080px] flex-col">
    <div className="flex shrink-0 items-center gap-5 border-stone-800 border-b bg-[#17140f] px-5 py-3">
      <div>
        <div className="font-serif text-lg text-stone-100">
          Edge-proportional sizing
        </div>
        <div className="mt-1 font-mono text-[10px] text-stone-600">
          rendered from backend decisions and backend-owned positions
        </div>
      </div>
      <div className="ml-auto flex gap-5">
        <Fact label="Deployable" value={model.wallet.available} />
        <Fact label="Reserved" value={model.wallet.reserved} />
        <Fact label="Positions" value={model.positions.length.toString()} />
      </div>
    </div>
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
      <AllocationView model={model} />
      <div className="min-h-0 overflow-auto border-stone-800 border-l bg-[#17140f] p-3.5">
        <HealthPanel model={model} />
      </div>
    </div>
  </div>
);

export const SurfaceBody = ({
  surface,
  model,
  selectedSource,
  inspectorSource,
  onSelectKernel,
  onInspectKernel,
  onCloseInspect,
  onOpenInsight,
}: {
  surface: TerminalSurface;
  model: TerminalModel;
  selectedSource: string;
  inspectorSource: string | null;
  onSelectKernel: (source: string) => void;
  onInspectKernel: (source: string) => void;
  onCloseInspect: () => void;
  onOpenInsight: () => void;
}) => {
  if (surface === "signals") {
    return (
      <SignalSurface
        model={model}
        selectedSource={selectedSource}
        onSelect={onSelectKernel}
      />
    );
  }

  if (surface === "decisions") {
    return <DecisionSurface model={model} />;
  }

  if (surface === "xray") {
    return <XraySurface model={model} />;
  }

  if (surface === "cortex") {
    return <CortexSurface model={model} />;
  }

  if (surface === "allocation") {
    return <AllocationSurface model={model} />;
  }

  return (
    <DashboardSurface
      model={model}
      selectedSource={selectedSource}
      inspectorSource={inspectorSource}
      onInspect={onInspectKernel}
      onCloseInspect={onCloseInspect}
      onOpenInsight={onOpenInsight}
    />
  );
};
