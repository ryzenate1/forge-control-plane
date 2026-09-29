"use client";

/**
 * Deprecated. Use `MetricSeriesChart` from `@/components/admin/telemetry-ui`.
 *
 * Had no importers and rendered in raw Tailwind palette colours, so it did not
 * follow the theme. The canonical chart reads its colours from
 * `lib/design-tokens` and distinguishes "no telemetry reported" from zero.
 */

export { MetricSeriesChart, type SeriesKey, type SeriesPoint } from "@/components/admin/telemetry-ui";
