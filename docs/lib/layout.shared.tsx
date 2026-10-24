import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';

// The Dakiya Mail mark (same path as site/'s Logo.astro), monochrome via currentColor.
function Mark() {
  return (
    <svg width="20" height="20" viewBox="0 0 746 764" fill="none" aria-hidden="true">
      <path
        d="M146 182H600A56 56 0 0 1 656 238V526A56 56 0 0 1 600 582H146A56 56 0 0 1 90 526V238A56 56 0 0 1 146 182ZM146 232V300L373 468L600 300V232L373 400Z"
        fill="currentColor"
      />
    </svg>
  );
}

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      title: (
        <div className="flex items-center gap-2">
          <Mark />
          <span className="font-extrabold tracking-tight text-[15px]">Dakiya Mail</span>
          <span className="rounded-md border border-fd-border bg-fd-muted px-1.5 py-px text-[10px] font-medium uppercase tracking-[0.14em] text-fd-muted-foreground">
            Docs
          </span>
        </div>
      ),
    },
    githubUrl: 'https://github.com/bitan-del/dakiya-mail',
    links: [
      {
        text: 'dakiyamail.com',
        url: 'https://dakiyamail.com',
      },
      {
        text: 'Dashboard',
        url: 'https://app.dakiyamail.com',
      },
    ],
  };
}
