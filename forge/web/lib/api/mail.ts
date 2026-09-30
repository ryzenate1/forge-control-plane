import { fetchJSON, putJSON, postJSON } from "./http";

export type PanelMailSettings = {
  driver: string;
  smtpHost: string;
  smtpPort: number;
  smtpEncryption: string;
  smtpUsername: string;
  smtpPassword: string;
  mailFromAddress: string;
  mailFromName: string;
};

export type MailTrigger = {
  event: string;
  template: string;
  label: string;
};

export const MAIL_MASKED_SECRET = "********";

export async function getMailSettings(): Promise<PanelMailSettings> {
  // Backend returns raw settings with smtpPassword masked as "********" when set
  const res = await fetchJSON<PanelMailSettings | { data: PanelMailSettings }>("/admin/mail/settings");
  const maybeData = (res as { data: PanelMailSettings }).data;
  return maybeData ?? (res as PanelMailSettings);
}

export async function updateMailSettings(settings: PanelMailSettings): Promise<{ ok: boolean }> {
  // If password is masked value, backend preserves existing stored value
  return putJSON<{ ok: boolean }>("/admin/mail/settings", settings);
}

export async function testMail(recipient: string): Promise<{ ok: boolean; message?: string }> {
  return postJSON<{ ok: boolean; message?: string }>("/admin/mail/test", { recipient });
}

export async function listMailTriggers(): Promise<MailTrigger[]> {
  const res = await fetchJSON<{ triggers: MailTrigger[] } | { data: { triggers: MailTrigger[] } } | MailTrigger[]>("/admin/mail/triggers");
  if (Array.isArray(res)) return res;
  const withTriggers = res as { triggers: MailTrigger[] };
  if (Array.isArray(withTriggers.triggers)) return withTriggers.triggers;
  const withData = res as { data: { triggers: MailTrigger[] } };
  if (withData.data?.triggers) return withData.data.triggers;
  return [];
}
