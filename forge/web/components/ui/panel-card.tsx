"use client";

/** Thin adapters over the canonical card and section. */

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { ForgeCard, ForgeSection } from "@/components/ui/forge";

export function PanelCard({ title, icon: Icon, children, className }: { title: string; icon?: LucideIcon; children: ReactNode; className?: string }) {
  return (
    <ForgeCard
      className={className}
      icon={Icon ? <Icon aria-hidden="true" size={15} /> : undefined}
      title={title}
    >
      {children}
    </ForgeCard>
  );
}

export function PanelSection({ title, description, action, children, className }: { title: string; description?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <ForgeSection actions={action} className={className} description={description} title={title}>
      {children}
    </ForgeSection>
  );
}
