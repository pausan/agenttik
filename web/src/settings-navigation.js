export const SETTINGS_SECTIONS = [
  { id: "general", label: "General", icon: "i-lucide-settings", children: [
    { id: "appearance", label: "Appearance", icon: "i-lucide-palette" },
    { id: "profiles", label: "Profiles", icon: "i-lucide-users" },
    { id: "projects", label: "Projects", icon: "i-lucide-archive" },
    { id: "shortcuts", label: "Shortcuts", icon: "i-lucide-keyboard" },
  ] },
  { id: "orchestrator", label: "Orchestrator", icon: "i-lucide-network" },
  { id: "providers", label: "Providers", icon: "i-lucide-plug", group: true, children: [
    { id: "subscriptions", label: "Subscriptions", icon: "i-lucide-id-card" },
    { id: "api-providers", label: "API Providers", icon: "i-lucide-key-round" },
  ] },
  { id: "models", label: "Models", icon: "i-lucide-sparkles" },
  { id: "server", label: "Server", icon: "i-lucide-server" },
  { id: "help", label: "Help", icon: "i-lucide-circle-help" },
  { id: "about", label: "About", icon: "i-lucide-info" },
];

export function visibleSettings(filter, counts) {
  return SETTINGS_SECTIONS.flatMap((section) => {
    const children = section.children?.filter((child) => !filter || counts[child.id]);
    const count = (counts[section.id] || 0) + (children || []).reduce((sum, child) => sum + (counts[child.id] || 0), 0);
    return !filter || count ? [{ ...section, children, count }] : [];
  });
}
