"use client";

/**
 * Deprecated. Use `MetricSeriesChart` from `@/components/admin/telemetry-ui`.
 *
 * Had no importers. Kept as a re-export so there is one metric chart in the
 * codebase rather than four that disagree about what missing data looks like.
 */

export { MetricSeriesChart, type SeriesKey, type SeriesPoint } from "@/components/admin/telemetry-ui";
