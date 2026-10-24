// The Dakiya Mail geometric mark. Single-path, inherits color via currentColor so
// it works on light or dark surfaces. Mirrors the wordmark used on the
// marketing site and dashboard so the admin app shares one brand.

export function Logo({ className }: { className?: string }) {
    return (
        <svg
            className={className}
            viewBox="0 0 746 764"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            aria-hidden="true"
        >
            <path
                d="M146 182H600A56 56 0 0 1 656 238V526A56 56 0 0 1 600 582H146A56 56 0 0 1 90 526V238A56 56 0 0 1 146 182ZM146 232V300L373 468L600 300V232L373 400Z"
                fill="currentColor"
            />
        </svg>
    );
}
