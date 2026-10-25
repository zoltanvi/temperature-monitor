/**
 * Shared theme registry.
 * Adding a theme: add tokens in themes.css, then one entry here.
 */
(function (global) {
	const DEFAULT_THEME = "dark";

	const THEMES = [
		{ id: "light", label: "Light", family: "neutral", variant: "light", swatch: ["#f4f4f5", "#3b82f6"] },
		{ id: "light-ocean", label: "Light Ocean", family: "ocean", variant: "light", swatch: ["#eff6ff", "#2563eb"] },
		{ id: "light-mint", label: "Light Mint", family: "mint", variant: "light", swatch: ["#ecfdf5", "#10b981"] },
		{ id: "light-lavender", label: "Light Lavender", family: "lavender", variant: "light", swatch: ["#f5f3ff", "#8b5cf6"] },
		{ id: "light-coral", label: "Light Coral", family: "coral", variant: "light", swatch: ["#fff1f2", "#f43f5e"] },
		{ id: "light-amber", label: "Light Amber", family: "amber", variant: "light", swatch: ["#fffbeb", "#f59e0b"] },

		{ id: "dark", label: "Dark", family: "neutral", variant: "dark", swatch: ["#18181b", "#60a5fa"] },
		{ id: "dark-ocean", label: "Dark Ocean", family: "ocean", variant: "dark", swatch: ["#122038", "#38bdf8"] },
		{ id: "dark-mint", label: "Dark Mint", family: "mint", variant: "dark", swatch: ["#102820", "#34d399"] },
		{ id: "dark-lavender", label: "Dark Lavender", family: "lavender", variant: "dark", swatch: ["#1f1630", "#a78bfa"] },
		{ id: "dark-coral", label: "Dark Coral", family: "coral", variant: "dark", swatch: ["#2a121c", "#fb7185"] },
		{ id: "dark-amber", label: "Dark Amber", family: "amber", variant: "dark", swatch: ["#2a2010", "#fbbf24"] },

		{ id: "neon", label: "Neon", family: "neutral", variant: "neon", swatch: ["#0a0a0a", "#22d3ee"] },
		{ id: "neon-ocean", label: "Neon Ocean", family: "ocean", variant: "neon", swatch: ["#001018", "#00e5ff"] },
		{ id: "neon-mint", label: "Neon Mint", family: "mint", variant: "neon", swatch: ["#001408", "#39ff14"] },
		{ id: "neon-lavender", label: "Neon Lavender", family: "lavender", variant: "neon", swatch: ["#0c0018", "#d500f9"] },
		{ id: "neon-coral", label: "Neon Coral", family: "coral", variant: "neon", swatch: ["#140008", "#ff073a"] },
		{ id: "neon-amber", label: "Neon Amber", family: "amber", variant: "neon", swatch: ["#141000", "#ffea00"] },
	];

	const VALID = new Set(THEMES.map((t) => t.id));

	function resolveTheme(id) {
		return VALID.has(id) ? id : DEFAULT_THEME;
	}

	global.ThemeRegistry = {
		THEMES,
		DEFAULT_THEME,
		resolveTheme,
	};
})(typeof window !== "undefined" ? window : globalThis);
