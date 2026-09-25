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
        },
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
        },
        ink: "var(--ink)",
        steel: "var(--steel)",
        concrete: "var(--concrete)",
        paper: "var(--paper)",
        phosphor: "var(--phosphor)",
        fault: "var(--fault)",
      },
      borderRadius: {
        sm: "var(--radius-sm)",
        DEFAULT: "var(--radius)",
        lg: "var(--radius-lg)",
        full: "var(--radius-full)",
      },
      boxShadow: {
        card: "var(--shadow-card)",
        elevated: "var(--shadow-elevated)",
        dialog: "var(--shadow-dialog)",
      },
    },
  },
  plugins: [],
};

export default config;
