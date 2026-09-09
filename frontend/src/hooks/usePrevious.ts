import React from "react";

const usePrevious = <T>(value: T) => {
	const ref = React.useRef<T | null>(null);
	React.useEffect(() => {
		ref.current = value;
	}, [value]);
	return ref.current as T | undefined;
};

export default usePrevious;
