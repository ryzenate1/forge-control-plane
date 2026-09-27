import type { ReactNode } from 'react';

interface FormCardProps {
  title: string;
  description?: string;
  children: ReactNode;
  actions?: ReactNode;
}

export function FormCard({ title, description, children, actions }: FormCardProps) {
  return (
    <div className="ui-card !p-0">
      <div className="border-b border-line px-6 py-4">
        <h3 className="t-section text-text">{title}</h3>
        {description && <p className="t-meta mt-1">{description}</p>}
      </div>
      <div className="space-y-4 px-6 py-4">{children}</div>
      {actions && (
        <div className="flex justify-end gap-2 border-t border-line px-6 py-4">{actions}</div>
      )}
    </div>
  );
}
