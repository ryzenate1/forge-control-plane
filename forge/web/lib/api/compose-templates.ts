import { deleteJSON, fetchJSON, patchJSON, postJSON } from './http';

// Portainer-style custom stack templates: reusable, parameterized compose
// documents that admins save and instantiate into real compose stacks.

export type ComposeTemplateParameterType =
  | 'text'
  | 'number'
  | 'select'
  | 'password'
  | 'env-file';

export interface ComposeTemplateParameter {
  key: string;
  label?: string;
  type: ComposeTemplateParameterType | string;
  default?: string;
  required: boolean;
  options?: string[];
  secret?: boolean;
}

export type ComposeTemplateVisibility = 'private' | 'public';

export interface ComposeTemplate {
  id: string;
  name: string;
  description?: string;
  category?: string;
  logoUrl?: string;
  composeYaml: string;
  parameters: ComposeTemplateParameter[];
  visibility: ComposeTemplateVisibility | string;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ComposeTemplateInstance {
  id: string;
  templateId: string;
  stackId: string;
  values: Record<string, string>;
  createdAt: string;
}

export interface ComposeTemplateInstantiateResult {
  instance: ComposeTemplateInstance;
  stack: unknown;
}

export interface ComposeTemplateCreateInput {
  name: string;
  description?: string;
  category?: string;
  logoUrl?: string;
  composeYaml: string;
  parameters?: ComposeTemplateParameter[];
  visibility?: ComposeTemplateVisibility;
}

export interface ComposeTemplatePreviewInput {
  templateId?: string;
  composeYaml?: string;
  parameters?: ComposeTemplateParameter[];
  values?: Record<string, string>;
}

export function listComposeTemplates() {
  return fetchJSON<ComposeTemplate[]>('/admin/compose-templates');
}

export function getComposeTemplate(id: string) {
  return fetchJSON<ComposeTemplate>(`/admin/compose-templates/${encodeURIComponent(id)}`);
}

export function createComposeTemplate(body: ComposeTemplateCreateInput) {
  return postJSON<ComposeTemplate>('/admin/compose-templates', body);
}

export function updateComposeTemplate(id: string, body: Partial<ComposeTemplateCreateInput>) {
  return patchJSON<ComposeTemplate>(`/admin/compose-templates/${encodeURIComponent(id)}`, body);
}

export function deleteComposeTemplate(id: string) {
  return deleteJSON<void>(`/admin/compose-templates/${encodeURIComponent(id)}`);
}

export function previewComposeTemplate(body: ComposeTemplatePreviewInput) {
  return postJSON<{ composeYaml: string }>('/admin/compose-templates/preview', body);
}

export function instantiateComposeTemplate(
  id: string,
  body: { name: string; nodeId?: string; values?: Record<string, string> },
) {
  return postJSON<ComposeTemplateInstantiateResult>(
    `/admin/compose-templates/${encodeURIComponent(id)}/instantiate`,
    body,
  );
}
