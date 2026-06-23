export type TerminalTone = "good" | "warn" | "bad" | "muted" | "info";

export const toneClasses = (tone: TerminalTone): string => {
  switch (tone) {
    case "good":
      return "border-emerald-400/35 bg-emerald-400/10 text-emerald-200";
    case "warn":
      return "border-amber-400/35 bg-amber-400/10 text-amber-200";
    case "bad":
      return "border-rose-400/35 bg-rose-400/10 text-rose-200";
    case "info":
      return "border-cyan-400/35 bg-cyan-400/10 text-cyan-200";
    default:
      return "border-stone-500/35 bg-stone-500/10 text-stone-300";
  }
};
