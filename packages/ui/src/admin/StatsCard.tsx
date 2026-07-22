import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

interface StatsCardProps {
  title: string;
  value: string | number;
  icon?: ReactNode;
  description?: string;
  trend?: { value: number; positive: boolean };
}

export function StatsCard({ title, value, icon, description, trend }: StatsCardProps) {
  return (
    <div className="ui-card">
      <div className="flex items-center justify-between">
        <div>
          <p className="t-eyebrow">{title}</p>
          <p className="t-readout mt-1 text-text">{value}</p>
          {description && <p className="t-meta mt-1">{description}</p>}
          {trend && (
            <p className={cn('mt-2 text-sm', trend.positive ? 'text-ok' : 'text-danger')}>
              {trend.positive ? '↑' : '↓'} {Math.abs(trend.value)}%
            </p>
          )}
        </div>
        {icon && <div className="text-text-muted">{icon}</div>}
      </div>
    </div>
  );
}
