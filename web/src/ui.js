/* Nuxt UI's pill tabs default to a filled primary indicator. The side and
   tab strips want the quieter version — a raised neutral pill — so both
   pass this. */
export const SEGMENTED = {
  indicator: "bg-default shadow-xs",
  trigger: "data-[state=active]:text-highlighted",
};
