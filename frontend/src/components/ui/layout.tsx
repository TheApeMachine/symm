export const Layout = ({ children }: { children: React.ReactNode }) => {
    return (
        <body className="flex h-full min-h-svh flex-col">
            {children}
        </body>
    )
};

Layout.Main = ({ children }: { children: React.ReactNode }) => {
	return <main className="flex">{children}</main>;
};

Layout.Nav = ({ children }: { children: React.ReactNode }) => {
	return <nav className="flex">{children}</nav>;
};

Layout.Aside = ({ children }: { children: React.ReactNode }) => {
	return <aside className="flex">{children}</aside>;
};

Layout.Footer = ({ children }: { children: React.ReactNode }) => {
	return <footer className="flex">{children}</footer>;
};

Layout.Header = ({ children }: { children: React.ReactNode }) => {
	return <header className="flex">{children}</header>;
};
