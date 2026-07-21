export function cn(...inputs: (string | false | null | undefined | Record<string, boolean | undefined | null>)[]): string {
  const classes: string[] = [];
  for (const input of inputs) {
    if (!input) continue;
    if (typeof input === "string") {
      classes.push(input);
    } else {
      for (const [key, value] of Object.entries(input)) {
        if (value) classes.push(key);
      }
    }
  }
  return classes.join(" ");
}

export function formatBytes(value: number, decimals = 1) {
  if (!Number.isFinite(value) || value < 0) return "Unavailable";
  if (value === 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const unit = Math.min(Math.floor(Math.log(Math.abs(value)) / Math.log(1024)), units.length - 1);
  const amount = value / 1024 ** unit;
  return `${amount.toFixed(unit === 0 ? 0 : decimals)} ${units[unit]}`;
}

export function formatDate(value?: string | number | Date | null, fallback = "Never") {
  if (value === undefined || value === null || value === "") return fallback;
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.valueOf())) return "Unknown";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function errorMessage(error: unknown, fallback = "An unexpected error occurred.") {
  return error instanceof Error && error.message ? error.message : fallback;
}
