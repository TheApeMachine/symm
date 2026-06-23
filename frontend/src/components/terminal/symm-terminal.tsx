import { type CSSProperties, useEffect, useState } from "react";
import type { TerminalSurface } from "#/components/terminal/model";
import { useTerminalModel } from "#/components/terminal/model";
import { CommandPalette } from "#/components/terminal/palette";
import { TerminalNav, TerminalTopBar } from "#/components/terminal/panels";
import { SurfaceBody, SurfaceHeader } from "#/components/terminal/surfaces";

export const SymmTerminal = () => {
  const [surface, setSurface] = useState<TerminalSurface>("dashboard");
  const [selectedSource, setSelectedSource] = useState("pumpdump");
  const [inspectorSource, setInspectorSource] = useState<string | null>(null);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [paletteQuery, setPaletteQuery] = useState("");
  const [paletteIndex, setPaletteIndex] = useState(0);
  const model = useTerminalModel();

  const selectSource = (source: string) => setSelectedSource(source);
  const inspectSource = (source: string) => {
    setSelectedSource(source);
    setInspectorSource(source);
  };
  const runPalette = (nextSurface: TerminalSurface, source?: string) => {
    if (source) {
      inspectSource(source);
    }

    setSurface(nextSurface);
    setPaletteOpen(false);
  };

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
        setPaletteQuery("");
        setPaletteIndex(0);
        return;
      }

      if (!paletteOpen) {
        return;
      }

      if (event.key === "Escape") {
        setPaletteOpen(false);
        return;
      }

      if (event.key === "ArrowDown") {
        event.preventDefault();
        setPaletteIndex((index) => index + 1);
        return;
      }

      if (event.key === "ArrowUp") {
        event.preventDefault();
        setPaletteIndex((index) => Math.max(0, index - 1));
      }
    };

    window.addEventListener("keydown", onKey);

    return () => window.removeEventListener("keydown", onKey);
  }, [paletteOpen]);

  return (
    <div
      className="fixed inset-0 z-50 flex min-h-0 flex-col overflow-hidden bg-[#0e0c0a] text-[13px] text-[#cbc2b4]"
      style={terminalVars}
    >
      <TerminalTopBar
        model={model}
        onOpenPalette={() => {
          setPaletteOpen(true);
          setPaletteQuery("");
          setPaletteIndex(0);
        }}
      />
      <div className="flex min-h-0 flex-1">
        <TerminalNav active={surface} model={model} onSelect={setSurface} />
        <main className="min-w-0 flex-1 overflow-auto bg-[#0e0c0a]">
          <SurfaceHeader
            title={surfaceLabel(surface)}
            meta={`${model.engine.sequence} - ${model.engine.measurements} measurements`}
          />
          <div className="h-[calc(100%_-_2.5rem)] min-h-[720px]">
            <SurfaceBody
              surface={surface}
              model={model}
              selectedSource={selectedSource}
              inspectorSource={inspectorSource}
              onSelectKernel={selectSource}
              onInspectKernel={inspectSource}
              onCloseInspect={() => setInspectorSource(null)}
              onOpenInsight={() => setSurface("signals")}
            />
          </div>
        </main>
      </div>
      <CommandPalette
        open={paletteOpen}
        query={paletteQuery}
        model={model}
        activeSurface={surface}
        activeIndex={paletteIndex}
        onQuery={setPaletteQuery}
        onClose={() => setPaletteOpen(false)}
        onRun={runPalette}
      />
    </div>
  );
};

const terminalVars = {
  "--acc": "#e8a33d",
  "--up": "#9cc06e",
  "--down": "#d5786a",
  "--info": "#7fbacb",
  "--warn": "#e8a33d",
  "--bg": "#0e0c0a",
  "--surface": "#17140f",
  "--raised": "#1f1a14",
  "--sunken": "#0a0907",
  "--line": "#2b251e",
  "--line2": "#3a342b",
  "--f1": "#f4efe5",
  "--f2": "#cbc2b4",
  "--f3": "#938a7e",
  "--f4": "#5f584e",
} as CSSProperties;

const surfaceLabel = (surface: TerminalSurface): string => {
  if (surface === "signals") {
    return "Signal insight";
  }

  if (surface === "decisions") {
    return "Decision tree";
  }

  if (surface === "xray") {
    return "Latent x-ray";
  }

  if (surface === "cortex") {
    return "Cognitive tree";
  }

  if (surface === "allocation") {
    return "Allocation";
  }

  return "Dashboard";
};
