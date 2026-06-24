export const Logo = () => {
	return (
		<>
			<svg
				width={22}
				height={22}
				viewBox="0 0 22 22"
				aria-label="SYMM"
				fill="none"
				style={{ display: "block" }}
			>
				<title>SYMM</title>
				<circle
					cx="11"
					cy="11"
					r="8.5"
					stroke="var(--acc)"
					stroke-width="1.3"
				></circle>
				<circle
					cx="11"
					cy="11"
					r="3.4"
					stroke="var(--acc)"
					stroke-width="1.3"
				></circle>
				<path
					d="M11 0.5V5M11 17V21.5M0.5 11H5M17 11H21.5"
					stroke="var(--acc)"
					stroke-width="1.3"
				></path>
			</svg>
			<span className="logo">SYMM</span>
		</>
	);
};
