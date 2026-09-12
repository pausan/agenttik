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
