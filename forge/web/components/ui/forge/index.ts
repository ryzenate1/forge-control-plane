/**
 * The canonical Forge design system.
 *
 * Everything the product UI needs comes from here. The older barrels
 * (`components/ui/primitives.tsx`, `components/admin/admin-ui.tsx`,
 * `components/ui/forge-primitives.tsx`) are thin adapters over this layer and
 * exist only so existing call sites keep compiling — new code imports from
 * `@/components/ui/forge`.
 *
 * Layers:
 *  - `status`   status vocabulary and tone resolution (no JSX)
 *  - `layout`   page, header, breadcrumbs, section, grid
 *  - `surface`  card, panel, well, metric, key/value
 *  - `controls` button, icon button, segmented, switch
 *  - `form`     field, input, select, combobox, search
 *  - `data`     table, pagination, data list
 *  - `feedback` badge, status, alert, empty/loading/error, progress, tooltip
 *  - `overlay`  dialog, confirm, drawer, dropdown, tabs, command palette
 *  - `code`     inline code, code block, command, terminal, log line
 *  - `chart`    chart frame, sparkline, bar series, gauge
 */

export * from "./status";
export * from "./layout";
export * from "./surface";
export * from "./controls";
export * from "./form";
export * from "./data";
export * from "./feedback";
export * from "./overlay";
export * from "./code";
export * from "./chart";
