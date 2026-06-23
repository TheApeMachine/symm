export const Fact = ({ label, value }: { label: string; value: string }) => (
  <div className="flex items-center justify-between gap-4 rounded-md border border-stone-800 bg-black/25 px-3 py-2">
    <span className="text-stone-600">{label}</span>
    <span className="truncate text-stone-200">{value}</span>
  </div>
);
