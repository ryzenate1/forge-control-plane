import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
    "./stores/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ["var(--font-sans)", "Manrope", "system-ui", "sans-serif"],
        display: ["var(--font-display)", "Space Grotesk", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "JetBrains Mono", "Fira Code", "Cascadia Code", "monospace"],
      },
      colors: {
        canvas: "var(--canvas)",
        nav: "var(--nav)",
        surface: {
          DEFAULT: "var(--surface)",
          base: "var(--canvas)",
          secondary: "var(--surface)",
          card: "var(--surface)",
          "card-header": "var(--surface-raised)",
          elevated: "var(--surface-raised)",
          raised: "var(--surface-raised)",
          input: "var(--surface-input)",
          hover: "var(--surface-hover)",
          well: "var(--surface-well)",
        },
        // `bg-well` is the short alias used by `.ui-well`.
        well: "var(--surface-well)",
        // Theme-aware overlays: a foreground wash that works on light and dark.
        overlay: {
          DEFAULT: "var(--overlay)",
          subtle: "var(--overlay-subtle)",
          strong: "var(--overlay-strong)",
        },
        border: {
          DEFAULT: "var(--border)",
          strong: "var(--border-strong)",
        },
        line: {
          DEFAULT: "var(--line)",
          strong: "var(--line-strong)",
        },
        text: {
          DEFAULT: "var(--text)",
          subtle: "var(--text-subtle)",
          muted: "var(--text-muted)",
          focus: "var(--focus)",
        },
        brand: {
          DEFAULT: "var(--brand)",
          hover: "var(--brand-hover)",
          dark: "var(--brand-dark)",
          subtle: "var(--brand-subtle)",
          line: "var(--brand-line)",
        },
        // Status vocabulary. `unknown` is deliberately not green — an
        // unreported reading must never read as healthy.
        ok: {
          DEFAULT: "var(--ok)",
          subtle: "var(--ok-subtle)",
          line: "var(--ok-line)",
        },
        warn: {
          DEFAULT: "var(--warn)",
          subtle: "var(--warn-subtle)",
          line: "var(--warn-line)",
        },
        info: {
          DEFAULT: "var(--info)",
          subtle: "var(--info-subtle)",
          line: "var(--info-line)",
        },
        unknown: {
          DEFAULT: "var(--unknown)",
          subtle: "var(--unknown-subtle)",
          line: "var(--unknown-line)",
        },
        // Legacy aliases kept so existing `--success`/`--warning` callers work.
        success: {
          DEFAULT: "var(--success)",
          subtle: "var(--success-subtle)",
        },
        warning: {
          DEFAULT: "var(--warning)",
          subtle: "var(--warning-subtle)",
        },
        danger: {
          DEFAULT: "var(--danger)",
          subtle: "var(--danger-subtle)",
          line: "var(--danger-line)",
        },
        ink: "var(--ink)",
        steel: "var(--steel)",
        concrete: "var(--concrete)",
        paper: "var(--paper)",
        phosphor: "var(--phosphor)",
        fault: "var(--fault)",
      },
      // Type scale. Forge is a dense operations tool, so the scale is tuned
      // one step tighter than Tailwind's defaults and every step is named —
      // there is no reason to reach for `text-[11px]` again.
      fontSize: {
        eyebrow: ["10px", { lineHeight: "16px", letterSpacing: "0.14em" }],
        meta: ["11px", { lineHeight: "16px" }],
        xs: ["12px", { lineHeight: "18px" }],
        sm: ["13px", { lineHeight: "20px" }],
        base: ["14px", { lineHeight: "22px" }],
        title: ["15px", { lineHeight: "22px", letterSpacing: "-0.015em" }],
        lg: ["17px", { lineHeight: "24px", letterSpacing: "-0.015em" }],
        heading: ["20px", { lineHeight: "26px", letterSpacing: "-0.02em" }],
        xl: ["20px", { lineHeight: "26px", letterSpacing: "-0.02em" }],
        display: ["26px", { lineHeight: "32px", letterSpacing: "-0.025em" }],
        "2xl": ["22px", { lineHeight: "28px", letterSpacing: "-0.022em" }],
        "3xl": ["26px", { lineHeight: "32px", letterSpacing: "-0.025em" }],
        "4xl": ["32px", { lineHeight: "38px", letterSpacing: "-0.03em" }],
      },
      // Three radii, nothing else. The larger Tailwind steps deliberately
      // collapse onto --radius-lg so `rounded-2xl` can no longer produce a
      // chunky decorative card.
      borderRadius: {
        sm: "var(--radius-xs)",
        DEFAULT: "var(--radius-sm)",
        md: "var(--radius-sm)",
        lg: "var(--radius)",
        xl: "var(--radius-lg)",
        "2xl": "var(--radius-lg)",
        "3xl": "var(--radius-lg)",
        full: "var(--radius-full)",
      },
      maxWidth: {
        page: "1600px",
        prose: "68ch",
      },
      transitionTimingFunction: {
        forge: "cubic-bezier(0.2, 0, 0, 1)",
        overlay: "cubic-bezier(0.16, 1, 0.3, 1)",
      },
      ringColor: {
        DEFAULT: "color-mix(in srgb, var(--brand) 22%, transparent)",
      },
      boxShadow: {
        flat: "var(--shadow-flat)",
        card: "var(--shadow-card)",
        raised: "var(--shadow-raised)",
        elevated: "var(--shadow-elevated)",
        popover: "var(--shadow-popover)",
        dialog: "var(--shadow-dialog)",
      },
    },
  },
  plugins: [],
};

export default config;
