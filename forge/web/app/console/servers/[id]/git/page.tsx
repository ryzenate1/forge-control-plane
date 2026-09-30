"use client";

/**
 * Console Git tab.
 *
 * The git UI is a single self-contained page that wraps itself in
 * `ServerConsoleLayout`. Inside `/console/servers/[id]/**` the parent layout
 * already owns the shell and publishes `ServerProvider`, so that wrapper
 * collapses to bare content and the page renders correctly in both trees.
 * Re-exporting keeps one implementation instead of a forked copy that drifts.
 */
export { default } from "../../../../server/[id]/git/page";
