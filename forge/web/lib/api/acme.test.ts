import { describe, it, expect, afterEach, vi } from "vitest";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import {
  listAcmeAccounts,
  createAcmeAccount,
  updateAcmeAccount,
  listDNSAccounts,
  uploadCertificate,
  exportCertificate,
  issueCertificate,
  renewCertificate,
  importDomainCertificate,
  attachCertificateToDomain,
} from "@/lib/api/acme";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("acme api client (real envelope shapes)", () => {
  it("unwraps { data } lists for accounts and dns-accounts", async () => {
    mockFetchByUrl({
      "/acme/accounts": jsonResponse({ data: [{ id: "a1", email: "admin@example.com", caUrl: "https://acme-v02.api.letsencrypt.org/directory", isDefault: true, createdAt: "2026-01-01" }] }),
      "/acme/dns-accounts": jsonResponse({ data: [{ id: "d1", name: "cf", provider: "cloudflare", createdAt: "2026-01-01" }] }),
    });
    const accounts = await listAcmeAccounts();
    expect(accounts).toHaveLength(1);
    expect(accounts[0].email).toBe("admin@example.com");
    const dns = await listDNSAccounts();
    expect(dns).toHaveLength(1);
    expect(dns[0].provider).toBe("cloudflare");
  });

  it("unwraps single-object envelopes on create/update", async () => {
    const { calls } = mockFetchByUrl({
      "/acme/accounts": {
        method: "POST",
        response: jsonResponse({ data: { id: "a2", email: "ops@example.com", caUrl: "https://acme-staging-v02.api.letsencrypt.org/directory", isDefault: false, createdAt: "2026-01-02" } }, 201),
      },
    });
    const created = await createAcmeAccount({ email: "ops@example.com", caUrl: "https://acme-staging-v02.api.letsencrypt.org/directory" });
    expect(created.id).toBe("a2");
    expect(calls[0].url).toContain("/acme/accounts");
  });

  it("sets default via PUT isDefault", async () => {
    const { calls } = mockFetchByUrl({
      "/acme/accounts/a1": {
        method: "PUT",
        response: jsonResponse({ data: { id: "a1", email: "admin@example.com", caUrl: "x", isDefault: true, createdAt: "2026-01-01" } }),
      },
    });
    const updated = await updateAcmeAccount("a1", { isDefault: true });
    expect(updated.isDefault).toBe(true);
    expect(JSON.parse(String(calls[0].init.body))).toMatchObject({ isDefault: true });
  });

  it("unwraps upload/export envelopes", async () => {
    mockFetchByUrl({
      "/certificates/upload": {
        method: "POST",
        response: jsonResponse({ data: { id: "c1", domains: ["example.com"], provider: "manual" } }, 201),
      },
      "/certificates/c1/export": {
        method: "POST",
        response: jsonResponse({ data: { certificate: "CERT", privateKey: "KEY" } }),
      },
    });
    const cert = await uploadCertificate("CERT", "KEY");
    expect(cert.id).toBe("c1");
    const exported = await exportCertificate("c1");
    expect(exported).toMatchObject({ certificate: "CERT", privateKey: "KEY" });
  });

  it("issues and renews through { data } envelopes", async () => {
    mockFetchByUrl({
      "/certificates/issue": {
        method: "POST",
        response: jsonResponse({ data: { id: "c2", domains: ["a.example.com"], provider: "letsencrypt" } }, 201),
      },
      "/certificates/c2/renew": {
        method: "POST",
        response: jsonResponse({ data: { id: "c2", domains: ["a.example.com"], provider: "letsencrypt" } }),
      },
    });
    const issued = await issueCertificate({ domains: ["a.example.com"], challengeType: "http-01", autoRenew: true });
    expect(issued.domains).toEqual(["a.example.com"]);
    const renewed = await renewCertificate("c2");
    expect(renewed.id).toBe("c2");
  });

  it("imports domain-bound certs and attaches LE to a domain", async () => {
    const { calls } = mockFetchByUrl({
      "/certificates": {
        method: "POST",
        response: jsonResponse({ data: { id: "c3", domains: ["b.example.com"], provider: "custom" } }, 201),
      },
      "/domains/dom1": {
        method: "PUT",
        response: jsonResponse({ data: { id: "dom1", certType: "letsencrypt" } }),
      },
    });
    const imported = await importDomainCertificate({ domainId: "dom1", certificate: "C", privateKey: "K" });
    expect(imported.id).toBe("c3");
    await attachCertificateToDomain("dom1", { certType: "letsencrypt", autoRenew: true, https: true });
    const putCall = calls.find((c) => c.url.includes("/domains/dom1"));
    expect(JSON.parse(String(putCall?.init.body))).toMatchObject({ certType: "letsencrypt" });
  });
});
