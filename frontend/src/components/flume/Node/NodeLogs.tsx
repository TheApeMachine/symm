"use client";

import {
	CheckIcon,
	CopyIcon,
	Maximize2Icon,
	TerminalIcon,
	XIcon,
} from "lucide-react";
import { useMemo, useState } from "react";
import type { NodeLogEntry, NodeStatus } from "#/components/flume/context";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Modal } from "#/components/ui/modal";
import { cn } from "@/lib/utils";

interface NodeLogsProps {
	nodeId: string;
	label: string;
	logs: NodeLogEntry[];
	status: NodeStatus;
	isOpen: boolean;
	onClose: () => void;
}

const formatTime = (ts: number) => {
	if (!ts) return "--:--:--";
	const d = new Date(ts);
	return (
		d.toTimeString().split(" ")[0] +
		"." +
		String(d.getMilliseconds()).padStart(3, "0")
	);
};

export const NodeLogs = ({
	nodeId,
	label,
	logs,
	status,
	isOpen,
	onClose,
}: NodeLogsProps) => {
	const [filter, setFilter] = useState<"all" | "info" | "warn" | "error">("all");
	const [isFullscreen, setIsFullscreen] = useState(false);
	const [copied, setCopied] = useState(false);

	const counts = useMemo(() => {
		const c = { info: 0, warn: 0, error: 0 };
		for (const log of logs) {
			if (log.level in c) c[log.level]++;
		}
		return c;
	}, [logs]);

	// Logs arrive as whole snapshots without ids, so an entry's position in
	// its node's log is its identity; filtering must not renumber it.
	const positionedLogs = useMemo(
		() => logs.map((entry, position) => ({ entry, position })),
		[logs],
	);

	const filteredRows = useMemo(() => {
		if (filter === "all") return positionedLogs;
		return positionedLogs.filter((row) => row.entry.level === filter);
	}, [positionedLogs, filter]);

	const filteredLogs = useMemo(
		() => filteredRows.map((row) => row.entry),
		[filteredRows],
	);

	const handleCopy = () => {
		const text = filteredLogs
			.map(
				(l) =>
					`[${formatTime(l.timestamp)}] [${l.level.toUpperCase()}] ${l.message}`,
			)
			.join("\n");

		if (typeof navigator !== "undefined" && navigator.clipboard) {
			navigator.clipboard.writeText(text);
			setCopied(true);
			setTimeout(() => setCopied(false), 2000);
		}
	};

	if (!isOpen) {
		return null;
	}

	return (
		<>
			<div
				className="border-t border-(--line) bg-(--sunken) p-2 flex flex-col gap-2 min-w-0"
				data-flume-node-logs={nodeId}
			>
				{/* Top bar: filters and fullscreen button */}
				<div className="flex items-center justify-between gap-1 border-b border-(--line2) pb-1.5 text-[10px]">
					<div className="flex items-center gap-1">
						<button
							type="button"
							onClick={() => setFilter("all")}
							className={cn(
								"px-1.5 py-0.5 rounded font-mono uppercase transition-colors",
								filter === "all"
									? "bg-(--acc)/20 text-(--acc) font-medium"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							All ({logs.length})
						</button>
						<button
							type="button"
							onClick={() => setFilter("info")}
							className={cn(
								"px-1.5 py-0.5 rounded font-mono uppercase transition-colors",
								filter === "info"
									? "bg-(--info)/20 text-(--info) font-medium"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							Info ({counts.info})
						</button>
						<button
							type="button"
							onClick={() => setFilter("warn")}
							className={cn(
								"px-1.5 py-0.5 rounded font-mono uppercase transition-colors",
								filter === "warn"
									? "bg-(--warning)/20 text-(--warning) font-medium"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							Warn ({counts.warn})
						</button>
						<button
							type="button"
							onClick={() => setFilter("error")}
							className={cn(
								"px-1.5 py-0.5 rounded font-mono uppercase transition-colors",
								filter === "error"
									? "bg-(--error)/20 text-(--error) font-medium"
									: "text-(--f4) hover:text-(--f2)",
							)}
						>
							Err ({counts.error})
						</button>
					</div>

					<div className="flex items-center gap-1">
						<button
							type="button"
							onClick={handleCopy}
							className="p-1 rounded text-(--f4) hover:text-(--f1) hover:bg-(--raised) transition-colors cursor-pointer"
							title={copied ? "Copied" : "Copy filtered logs"}
						>
							{copied ? (
								<CheckIcon className="size-3 text-emerald-400" />
							) : (
								<CopyIcon className="size-3" />
							)}
						</button>
						<button
							type="button"
							onClick={() => setIsFullscreen(true)}
							className="p-1 rounded text-(--f4) hover:text-(--f1) hover:bg-(--raised) transition-colors cursor-pointer"
							title="Expand logs to modal"
						>
							<Maximize2Icon className="size-3" />
						</button>
						<button
							type="button"
							onClick={onClose}
							className="p-1 rounded text-(--f4) hover:text-(--f1) hover:bg-(--raised) transition-colors cursor-pointer"
							title="Close log viewer"
						>
							<XIcon className="size-3" />
						</button>
					</div>
				</div>

				{/* Inline log list */}
				<div
					className="flex flex-col gap-1 max-h-44 overflow-y-auto font-mono text-[10.5px] leading-relaxed select-text"
					style={{ overscrollBehavior: "contain" }}
					onWheel={(e) => e.stopPropagation()}
				>
					{filteredLogs.length === 0 ? (
						<div className="py-3 text-center text-(--f4) italic text-[10px]">
							No logs recorded yet
						</div>
					) : (
						filteredRows.map(({ entry, position }) => (
							<div
								key={position}
								className={cn(
									"flex items-start gap-1.5 py-0.5 px-1 rounded hover:bg-(--raised)/40 break-all",
									entry.level === "error" && "text-red-300 bg-red-950/20",
									entry.level === "warn" && "text-amber-300 bg-amber-950/20",
									entry.level === "info" && "text-(--f2)",
								)}
							>
								<span className="text-(--f4) shrink-0 select-none text-[9.5px]">
									{formatTime(entry.timestamp)}
								</span>
								<span
									className={cn(
										"shrink-0 font-semibold text-[9px] uppercase px-1 rounded",
										entry.level === "error" && "bg-red-500/20 text-red-400",
										entry.level === "warn" && "bg-amber-500/20 text-amber-400",
										entry.level === "info" && "bg-(--acc)/15 text-(--acc)",
									)}
								>
									{entry.level}
								</span>
								<span className="flex-1 min-w-0">{entry.message}</span>
							</div>
						))
					)}
				</div>
			</div>

			{/* Fullscreen Inspector Modal */}
			{isFullscreen && (
				<Modal
					open={isFullscreen}
					onClose={() => setIsFullscreen(false)}
					size="lg"
				>
					<Modal.Header>
						<div className="flex items-center gap-2">
							<TerminalIcon className="size-4 text-(--acc)" />
							<span className="font-medium text-sm text-(--f1)">
								{label} ({nodeId}) Logs
							</span>
							<Badge
								variant={
									status === "ready" || status === "ok"
										? "success"
										: status === "busy"
											? "info"
											: status === "waiting"
												? "warning"
												: status === "error" || status === "fatal"
													? "error"
													: "disabled"
								}
								size="xxs"
								dot
								pulse={status === "busy"}
								label={status.toUpperCase()}
							/>
						</div>
					</Modal.Header>
					<Modal.Body className="max-h-[60vh] flex flex-col gap-3">
						<div className="flex items-center justify-between gap-2 border-b border-(--line) pb-2 text-xs">
							<div className="flex items-center gap-1.5">
								<button
									type="button"
									onClick={() => setFilter("all")}
									className={cn(
										"px-2 py-1 rounded font-mono text-xs uppercase",
										filter === "all"
											? "bg-(--acc)/20 text-(--acc) font-medium"
											: "text-(--f4) hover:text-(--f2)",
									)}
								>
									All ({logs.length})
								</button>
								<button
									type="button"
									onClick={() => setFilter("info")}
									className={cn(
										"px-2 py-1 rounded font-mono text-xs uppercase",
										filter === "info"
											? "bg-(--info)/20 text-(--info) font-medium"
											: "text-(--f4) hover:text-(--f2)",
									)}
								>
									Info ({counts.info})
								</button>
								<button
									type="button"
									onClick={() => setFilter("warn")}
									className={cn(
										"px-2 py-1 rounded font-mono text-xs uppercase",
										filter === "warn"
											? "bg-(--warning)/20 text-(--warning) font-medium"
											: "text-(--f4) hover:text-(--f2)",
									)}
								>
									Warn ({counts.warn})
								</button>
								<button
									type="button"
									onClick={() => setFilter("error")}
									className={cn(
										"px-2 py-1 rounded font-mono text-xs uppercase",
										filter === "error"
											? "bg-(--error)/20 text-(--error) font-medium"
											: "text-(--f4) hover:text-(--f2)",
									)}
								>
									Error ({counts.error})
								</button>
							</div>
							<div className="flex items-center gap-2">
								<button
									type="button"
									onClick={handleCopy}
									className="flex items-center gap-1 px-2 py-1 rounded text-xs text-(--f3) hover:text-(--f1) hover:bg-(--raised)"
								>
									{copied ? (
										<CheckIcon className="size-3 text-emerald-400" />
									) : (
										<CopyIcon className="size-3" />
									)}
									{copied ? "Copied" : "Copy"}
								</button>
							</div>
						</div>

						<div
							className="flex flex-col gap-1 overflow-y-auto font-mono text-xs leading-relaxed select-text p-2 rounded bg-(--sunken) border border-(--line2) min-h-[220px]"
							onWheel={(e) => e.stopPropagation()}
						>
							{filteredLogs.length === 0 ? (
								<div className="py-8 text-center text-(--f4) italic">
									No logs recorded yet
								</div>
							) : (
								filteredRows.map(({ entry, position }) => (
									<div
										key={position}
										className={cn(
											"flex items-start gap-2 py-1 px-1.5 rounded hover:bg-(--raised)/40 break-all",
											entry.level === "error" && "text-red-300 bg-red-950/20",
											entry.level === "warn" && "text-amber-300 bg-amber-950/20",
											entry.level === "info" && "text-(--f2)",
										)}
									>
										<span className="text-(--f4) shrink-0 select-none text-[11px]">
											{formatTime(entry.timestamp)}
										</span>
										<span
											className={cn(
												"shrink-0 font-semibold text-[10px] uppercase px-1.5 py-0.5 rounded",
												entry.level === "error" && "bg-red-500/20 text-red-400",
												entry.level === "warn" && "bg-amber-500/20 text-amber-400",
												entry.level === "info" && "bg-(--acc)/15 text-(--acc)",
											)}
										>
											{entry.level}
										</span>
										<span className="flex-1 min-w-0">{entry.message}</span>
									</div>
								))
							)}
						</div>
					</Modal.Body>
					<Modal.Footer>
						<Button variant="outline" onClick={() => setIsFullscreen(false)}>
							Close
						</Button>
					</Modal.Footer>
				</Modal>
			)}
		</>
	);
};
