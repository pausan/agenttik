/* Nuxt UI's pill tabs default to a filled primary indicator. The side and
   tab strips want the quieter version — a raised neutral pill — so both
   pass this. */
export const SEGMENTED = {
  indicator: "bg-default shadow-xs",
  trigger: "data-[state=active]:text-highlighted",
};

// Keep phone navigation and automatic keyboard focus on the same breakpoint.
export const MOBILE_QUERY = "(max-width: 767px)";
export const isMobile = () => window.matchMedia(MOBILE_QUERY).matches;

/* The classes of an extra-small ghost neutral icon UButton. Long lists use a
   plain button with these: a UButton's setup costs about 0.4 ms, so the
   hundreds a list of changes needs would hold up its first paint. */
export const ROW_BUTTON = "rounded-md p-1 inline-flex items-center text-default transition-colors hover:bg-elevated active:bg-elevated outline-inverted/25 focus-visible:outline-3 disabled:cursor-not-allowed disabled:opacity-75 hover:disabled:bg-transparent";
