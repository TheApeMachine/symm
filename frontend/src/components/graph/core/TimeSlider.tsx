import {
	type Ref,
	useCallback,
	useEffect,
	useImperativeHandle,
	useRef,
	useState,
} from "react";

import "../styles/slider-styles.css";

export type TimeSliderHandle = {
	togglePlay: () => void;
	expandRange: () => void;
	toggleRepeat: () => void;
};

export interface TimeSliderProps {
	ref?: Ref<TimeSliderHandle>;
	min: number;
	max: number;
	from: number;
	to: number;
	onChange: (nextFrom: number, nextTo: number) => void;
	onPlaybackChange: (
		playing: boolean,
		repeating: boolean,
		expanded: boolean,
	) => void;
	showControls: boolean;
	legacyAppearance?: boolean;
	formatTime: (unixSeconds: number) => string;
}

const clamp = (value: number, lo: number, hi: number) =>
	Math.min(hi, Math.max(lo, value));

export const TimeSlider = ({
	ref: reference,
	min,
	max,
	from,
	to,
	onChange,
	onPlaybackChange,
	showControls,
	legacyAppearance,
	formatTime,
}: TimeSliderProps) => {
	const propsReference = useRef({
		from,
		to,
		min,
		max,
		onChange,
	});

	propsReference.current = { from, to, min, max, onChange };

	const [playing, setPlaying] = useState(false);
	const [repeating, setRepeating] = useState(false);
	const [expanded, setExpanded] = useState(false);
	const savedWindowReference = useRef<{ from: number; to: number } | null>(
		null,
	);

	const notifyPlayback = useCallback(() => {
		onPlaybackChange(playing, repeating, expanded);
	}, [onPlaybackChange, playing, repeating, expanded]);

	useEffect(() => {
		notifyPlayback();
	}, [notifyPlayback]);

	useEffect(() => {
		if (!playing) return undefined;

		const stride = (): number => {
			const span = propsReference.current.to - propsReference.current.from;
			if (!Number.isFinite(span) || span <= 0) return 1;
			const stepFraction = expanded ? span * 0.05 : span * 0.04;
			return Math.max(Number.EPSILON, stepFraction);
		};

		const intervalId = globalThis.window.setInterval(() => {
			const {
				from: currentFrom,
				to: currentTo,
				min: currentMin,
				max: currentMax,
				onChange: change,
			} = propsReference.current;

			const span = currentTo - currentFrom;
			if (!Number.isFinite(span) || span <= 0) return;

			const delta = stride();
			let nextFrom = currentFrom + delta;
			let nextTo = currentTo + delta;

			if (nextTo > currentMax + Number.EPSILON) {
				if (repeating) {
					nextFrom = currentMin;
					nextTo = currentMin + span;
					if (nextTo > currentMax) {
						nextTo = currentMax;
						nextFrom = currentMax - span;
					}
				} else {
					change(currentMax - span, currentMax);
					setPlaying(false);
					return;
				}
			}

			change(
				clamp(nextFrom, currentMin, currentMax - span),
				clamp(nextTo, currentMin + span, currentMax),
			);
		}, 250);

		return () => globalThis.window.clearInterval(intervalId);
	}, [playing, repeating, expanded]);

	useImperativeHandle(reference, () => ({
		togglePlay: () => setPlaying((value) => !value),

		toggleRepeat: () => setRepeating((value) => !value),

		expandRange: () => {
			const snapshot = propsReference.current;

			setExpanded((currentExpanded) => {
				if (!currentExpanded) {
					savedWindowReference.current = {
						from: snapshot.from,
						to: snapshot.to,
					};
					snapshot.onChange(snapshot.min, snapshot.max);
					return true;
				}

				const stored = savedWindowReference.current;

				if (stored) {
					snapshot.onChange(stored.from, stored.to);
				}

				savedWindowReference.current = null;
				return false;
			});
		},
	}));

	const handleFromSlide = useCallback(
		(raw: number) => {
			const epsilon = spanGuard(min, max);
			const upperFrom = Math.min(max, to - epsilon);
			const nextFrom = clamp(raw, min, upperFrom);

			onChange(nextFrom, to);
		},
		[min, max, onChange, to],
	);

	const handleToSlide = useCallback(
		(raw: number) => {
			const epsilon = spanGuard(min, max);
			const lowerTo = Math.max(min, from + epsilon);
			const nextTo = clamp(raw, lowerTo, max);

			onChange(from, nextTo);
		},
		[min, max, onChange, from],
	);

	const sliderStep = (): number =>
		clamp((max - min) / 500, Number.EPSILON, max - min || 1);

	return (
		<div
			className={
				legacyAppearance
					? "time-slider-root time-slider-root--legacy"
					: "time-slider-root"
			}
		>
			{showControls && (
				<div className="time-slider-meta">
					<span aria-live="polite">{formatTime(from)}</span>
					<span aria-live="polite">{formatTime(to)}</span>
				</div>
			)}
			<div className="time-slider-ranges">
				<label>
					<span className="sr-only">Window start</span>
					<input
						max={max}
						min={min}
						step={sliderStep()}
						type="range"
						value={from}
						onChange={(event) => {
							handleFromSlide(Number.parseFloat(event.target.value));
						}}
					/>
				</label>
				<label>
					<span className="sr-only">Window end</span>
					<input
						max={max}
						min={min}
						step={sliderStep()}
						type="range"
						value={to}
						onChange={(event) => {
							handleToSlide(Number.parseFloat(event.target.value));
						}}
					/>
				</label>
			</div>
		</div>
	);
};

function spanGuard(lower: number, upper: number): number {
	const span = upper - lower;
	if (!Number.isFinite(span)) return Number.EPSILON;
	const guard = span * 0.001;

	return clamp(guard, Number.EPSILON, span || Number.EPSILON);
}
