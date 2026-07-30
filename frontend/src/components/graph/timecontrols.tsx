import type { RefObject } from "react";
import type { TimeRange } from "./component";
import { TimeSlider, type TimeSliderHandle } from "./core/TimeSlider";

interface NodeGraphLegacyTimeControlsProps {
	showTimeSlider: boolean;
	timeRange: TimeRange;
	onTimeChange: (from: number, to: number) => void;
	onPlaybackChange: (
		isPlaying: boolean,
		isRepeating: boolean,
		isExpanded: boolean,
	) => void;
	sliderRef: RefObject<TimeSliderHandle | null>;
	formatTime: (value: number) => string;
	isPlaying: boolean;
	isRepeating: boolean;
	isExpanded: boolean;
	onTogglePlay: () => void;
	onExpandRange: () => void;
	onToggleRepeat: () => void;
}

export function NodeGraphLegacyTimeControls({
	showTimeSlider,
	timeRange,
	onTimeChange,
	onPlaybackChange,
	sliderRef,
	formatTime,
	isPlaying,
	isRepeating,
	isExpanded,
	onTogglePlay,
	onExpandRange,
	onToggleRepeat,
}: NodeGraphLegacyTimeControlsProps) {
	if (!showTimeSlider) return null;

	return (
		<div className="legacy-controls-container">
			<div style={{ width: "100%" }}>
				<TimeSlider
					ref={sliderRef}
					min={timeRange.min}
					max={timeRange.max}
					from={timeRange.from}
					to={timeRange.to}
					onChange={onTimeChange}
					onPlaybackChange={onPlaybackChange}
					showControls={false}
					legacyAppearance={true}
					formatTime={formatTime}
				/>
			</div>

			<div className="legacy-buttons">
				<button
					type="button"
					className="btn-legacy btn-primary-legacy"
					onClick={onTogglePlay}
					title={isPlaying ? "Pause" : "Play"}
				>
					{isPlaying ? (
						<span
							style={{
								display: "inline-block",
								height: "14px",
								width: "14px",
							}}
						>
							<svg
								viewBox="0 0 16 16"
								fill="currentColor"
								style={{ verticalAlign: "middle" }}
							>
								<title>Pause</title>
								<rect x="3" y="2" width="4" height="12" rx="1" />
								<rect x="9" y="2" width="4" height="12" rx="1" />
							</svg>
						</span>
					) : (
						<span
							className="glyphicon glyphicon-play"
							aria-hidden="true"
							style={{
								display: "inline-block",
								height: "14px",
								width: "14px",
							}}
						>
							<svg
								viewBox="0 0 16 16"
								fill="currentColor"
								style={{ verticalAlign: "middle" }}
							>
								<title>Play</title>
								<path d="M4 2.5v11a.5.5 0 0 0 .75.43l9-5.5a.5.5 0 0 0 0-.86l-9-5.5A.5.5 0 0 0 4 2.5z" />
							</svg>
						</span>
					)}
				</button>
				<button
					type="button"
					className="btn-legacy btn-primary-legacy"
					onClick={onExpandRange}
					title={isExpanded ? "Restore" : "Show All"}
				>
					{isExpanded ? (
						<span
							className="glyphicon glyphicon-minus"
							aria-hidden="true"
							style={{
								display: "inline-block",
								height: "14px",
								width: "14px",
							}}
						>
							<svg
								viewBox="0 0 16 16"
								fill="currentColor"
								style={{ verticalAlign: "middle" }}
							>
								<title>Collapse</title>
								<path d="M4 8a.5.5 0 0 1 .5-.5h7a.5.5 0 0 1 0 1h-7A.5.5 0 0 1 4 8z" />
							</svg>
						</span>
					) : (
						<span
							className="glyphicon glyphicon-plus"
							aria-hidden="true"
							style={{
								display: "inline-block",
								height: "14px",
								width: "14px",
							}}
						>
							<svg
								viewBox="0 0 16 16"
								fill="currentColor"
								style={{ verticalAlign: "middle" }}
							>
								<title>Expand</title>
								<path d="M8 4a.5.5 0 0 1 .5.5v3h3a.5.5 0 0 1 0 1h-3v3a.5.5 0 0 1-1 0v-3h-3a.5.5 0 0 1 0-1h3v-3A.5.5 0 0 1 8 4z" />
							</svg>
						</span>
					)}
				</button>
				<button
					type="button"
					className="btn-legacy btn-primary-legacy"
					onClick={onToggleRepeat}
					title="Repeat"
				>
					<span
						className={`glyphicon glyphicon-repeat ${!isRepeating ? "text-muted" : ""}`}
						aria-hidden="true"
						style={{
							display: "inline-block",
							height: "14px",
							width: "14px",
						}}
					>
						<svg
							viewBox="0 0 16 16"
							fill="currentColor"
							style={{ verticalAlign: "middle" }}
						>
							<title>Repeat</title>
							<path d="M11 5.466V4H5a4 4 0 0 0-4 4 .5.5 0 0 1-1 0 5 5 0 0 1 5-5h6V1.534a.25.25 0 0 1 .41-.192l2.36 1.966c.12.1.12.284 0 .384l-2.36 1.966a.25.25 0 0 1-.41-.192z" />
							<path d="M5 10.534V12h6a4 4 0 0 0 4-4 .5.5 0 0 1 1 0 5 5 0 0 1-5 5H5v1.466a.25.25 0 0 1-.41.192l-2.36-1.966a.25.25 0 0 1 0-.384l2.36-1.966a.25.25 0 0 1 .41.192z" />
						</svg>
					</span>
				</button>
			</div>
		</div>
	);
}
