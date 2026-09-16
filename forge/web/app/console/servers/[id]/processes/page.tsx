"use client";

/**
 * Console Processes tab.
 *
 * Re-exported rather than copied: the page wraps itself in
 * `ServerConsoleLayout`, which collapses to bare content under the console
 * layout's `ServerProvider`, so one implementation serves both route trees.
 */
export { default } from "../../../../server/[id]/processes/page";
