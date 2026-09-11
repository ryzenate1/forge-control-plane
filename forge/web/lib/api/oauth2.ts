import { requestJSON } from "./http";

export type OAuth2TokenRequest =
  | {
      grant_type: "client_credentials";
      client_id: string;
      client_secret: string;
      scope?: string;
    }
  | {
      grant_type: "authorization_code";
      client_id: string;
      client_secret?: string;
      code: string;
      redirect_uri?: string;
      code_verifier?: string;
    }
  | {
      grant_type: "refresh_token";
      client_id?: string;
      client_secret?: string;
      refresh_token: string;
      scope?: string;
    };

export type OAuth2TokenResponse = {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope?: string;
};

/**
 * POST /oauth2/token — RFC 6749 compliant token endpoint (canonical).
 * Backend: server.go v1.Post("/oauth2/token", IssueOAuth2Token) and alias /oauth/token.
 * RFC 6749 §4 requires `application/x-www-form-urlencoded`; the backend also
 * accepts JSON via `c.BodyParser`, but form-encoding is the interoperable
 * contract (and the only shape a generic OAuth client will send), so this
 * helper encodes as form data through the canonical primitive (same CSRF,
 * credentials, timeout and error shaping as every other call).
 */
export async function issueOAuth2Token(req: OAuth2TokenRequest): Promise<OAuth2TokenResponse> {
  const form = new URLSearchParams();
  for (const [key, value] of Object.entries(req)) {
    if (value !== undefined && value !== null) form.set(key, String(value));
  }
  return requestJSON<OAuth2TokenResponse>("/oauth2/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: form.toString(),
  });
}

/** @deprecated use issueOAuth2Token (POST /oauth2/token). Alias kept for backend compat. */
export async function issueOAuthTokenLegacy(req: OAuth2TokenRequest): Promise<OAuth2TokenResponse> {
  const form = new URLSearchParams();
  for (const [key, value] of Object.entries(req)) {
    if (value !== undefined && value !== null) form.set(key, String(value));
  }
  return requestJSON<OAuth2TokenResponse>("/oauth/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: form.toString(),
  });
}
