"use client";

/**
 * Deprecated. Use `MetricSeriesChart` from `@/components/admin/telemetry-ui`.
 *
 * The implementation this replaces mapped series values with
 * `(m as Record<string, number>)[metric] ?? 0`, which plots a metric no node
 * reported as a flat line at zero — a fleet that is reporting nothing renders
 * identically to a fleet that is completely idle. That is the zero-for-unknown
 * substitution the telemetry layer exists to prevent (AGENTS.md: "Unknown is
 * not zero, not-reported is not zero, and a stale reading is not a healthy
 * one"). `MetricSeriesChart` draws no series when there are no points and
 * overlays the reason instead.
 */

export { MetricSeriesChart, type SeriesKey, type SeriesPoint } from "@/components/admin/telemetry-ui";
