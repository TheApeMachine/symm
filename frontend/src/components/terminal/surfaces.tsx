import type { ReactNode } from "react";
import {
  TerminalCognitiveChart,
  TerminalManifoldChart,
  TerminalPositionChart,
  TerminalPredictionChart,
  TerminalResonanceChart,
  TerminalSignalHeatmap,
} from "#/components/terminal/charts";
import { DashboardSurface } from "#/components/terminal/dashboard";
import { DecisionTreeView } from "#/components/terminal/decision";
import { Fact } from "#/components/terminal/fact";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import type {
  TerminalModel,
  TerminalSurface,
} from "#/components/terminal/model";
import { TerminalSection } from "#/components/terminal/panels";
import { KernelList } from "#/components/terminal/rows";
import { AllocationView, SignalDetail } from "#/components/terminal/widgets";

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

const symbolsFromModel = (model: TerminalModel): string[] => {
  const symbols = Array.from(
    new Set(model.decisions.map((decision) => decision.symbol)),
  );

  if (symbols.length > 0) {
    return symbols.slice(0, 10);
  }

  if (model.cognitiveScopes.length > 0) {
    return model.cognitiveScopes.slice(0, 10);
  }

  return ["stream"];
};

const ContextStrip = ({
  label,
  symbols,
  meta,
}: {
  label: string;
  symbols: string[];
  meta?: string;
}) => (
  <div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-[var(--line)] border-b bg-[var(--surface)] px-3.5">
    <span className="mr-1 shrink-0 font-semibold text-[10px] text-[var(--f4)] uppercase tracking-[0.13em]">
      {label}
    </span>
    {symbols.map((symbol) => (
      <span
        key={symbol}
        className="shrink-0 rounded-[3px] border border-[var(--line)] bg-[var(--sunken)] px-2.5 py-1 font-mono text-[11px] text-[var(--f2)]"
      >
        {symbol}
      </span>
    ))}
    {meta ? (
      <span className="ml-auto shrink-0 font-mono text-[10px] text-[var(--f4)]">
        {meta}
      </span>
    ) : null}
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
  <div className="grid h-full min-w-[1120px] grid-cols-[264px_minmax(420px,1fr)_350px]">
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
        <div className="mb-2 font-semibold text-[10px] text-[var(--f4)] uppercase tracking-[0.13em]">
          Cross-section · confidence heatmap
        </div>
        <div className="h-56 overflow-hidden rounded-[3px] border border-[var(--line)]">
          <TerminalSignalHeatmap kind="confidence" />
        </div>
      </div>
    </div>
    <div className="min-h-0 space-y-3 overflow-auto border-[var(--line)] border-l bg-[var(--surface)] p-3.5">
      <HealthPanel model={model} />
      <RadarPanel model={model} />
      <div className="h-56 overflow-hidden rounded-[3px] border border-[var(--line)]">
        <TerminalSignalHeatmap kind="surprise" />
      </div>
    </div>
  </div>
);

const XraySurface = ({ model }: { model: TerminalModel }) => (
  <div className="flex h-full min-w-[1100px] flex-col">
    <ContextStrip label="Inspect symbol" symbols={symbolsFromModel(model)} />
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
      <div className="grid min-h-0 grid-rows-2 border-stone-800 border-r">
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
      <div className="grid min-h-0 grid-rows-2 bg-[#17140f]">
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
  </div>
);

const CortexSurface = ({ model }: { model: TerminalModel }) => (
  <div className="flex h-full min-w-[1140px] flex-col">
    <ContextStrip
      label="Sensory context"
      symbols={symbolsFromModel(model)}
      meta={`${model.cognitiveScopes.length} scopes`}
    />
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]">
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
            value={
              model.cognitive ? model.cognitive.entropyBits.toFixed(3) : "0"
            }
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
    return <DecisionTreeView model={model} />;
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
