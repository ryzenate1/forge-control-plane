"use client";

import { Box, Container, GitBranch, Layers } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { AppType } from "@/lib/api/apps";

/**
 * Shared app type -> icon mapping.
 * Previously duplicated as local `typeIcons` in:
 *  - app/admin/apps/page.tsx (local copy)
 *  - potential shadow vs EGG_TEMPLATES static map
 *
 * Centralized here so list/detail views share a single reference and
 * cannot diverge (Phase 03 finding: typeIcons divergence).
 */

export const APP_TYPE_ICONS: Record<AppType, LucideIcon> = {
  image: Box,
  git: GitBranch,
  compose: Container,
  game_server: Layers,
};

/** Alias kept for legacy import name `typeIcons` */
export const typeIcons = APP_TYPE_ICONS;

export function iconForAppType(type: AppType): LucideIcon {
  return APP_TYPE_ICONS[type] ?? Layers;
}
