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
        sans: ["Plus Jakarta Sans", "IBM Plex Sans", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Fira Code", "Cascadia Code", "monospace"],
      },
      colors: {
        surface: {
          base: "var(--canvas)",
          secondary: "var(--surface)",
          card: "var(--surface)",
          "card-header": "var(--surface-raised)",
          elevated: "var(--surface-raised)",
          input: "var(--surface-input)",
          sidebar: "#151920",
          "sidebar-hover": "#10141a",
          tab: "#1a1f2e",
        },
        brand: {
          DEFAULT: "#dc2626",
          hover: "#b91c1c",
          dark: "#991b1b",
          darker: "#7f1d1d",
          light: "#ef4444",
        },
        neutral: {
          border: "#374151",
          text: "#4b5563",
          muted: "#64748b",
          secondary: "#94a3b8",
        },
        semantic: {
          success: "#059669",
          warning: "#d97706",
          error: "#dc2626",
        },
        admin: {
          bg: "#e9eef4",
          card: "#fbfbfb",
          border: "#d2dde8",
        },
        panel: {
          brand: "#dc2626",
          ink: "#f1f5f9",
          muted: "#94a3b8",
          line: "#374151",
          surface: "#1e2536",
        },
      },
      borderRadius: {
        xl: "12px",
        "2xl": "16px",
      },
      boxShadow: {
        card: "0 4px 24px rgba(0, 0, 0, 0.3)",
      },
    },
  },
  plugins: [],
};

export default config;
