import type { ReactNode } from "react";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LoginPage from "@/app/page";
import { safeRedirectPath } from "@/components/ui/auth-utils";
import ForgotPasswordPage from "@/app/forgot-password/page";
import ResetPasswordPage from "@/app/reset-password/page";
import { useServerStore } from "@/stores/use-server-store";
import { jsonResponse, mockFetch, requestJSON } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";

const replace = vi.fn();
let search = new URLSearchParams();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push: vi.fn() }), usePathname: () => "/", useSearchParams: () => search }));
vi.mock("next/link", () => ({ default: ({ children, href, ...props }: { children: ReactNode; href: string }) => <a href={href} {...props}>{children}</a> }));

beforeEach(() => { replace.mockReset(); search = new URLSearchParams(); useServerStore.setState({ currentUser: null }); });

describe("safe post-login redirects", () => {
  it("accepts local paths and rejects external or protocol-relative destinations", () => {
    expect(safeRedirectPath("/account?tab=security")).toBe("/account?tab=security");
    expect(safeRedirectPath("//evil.example/path")).toBeNull();
    expect(safeRedirectPath("https://evil.example/path")).toBeNull();
    expect(safeRedirectPath("/\\evil.example")).toBeNull();
  });

  it("returns to a safe requested page after sign in", async () => {
    search = new URLSearchParams("next=%2Faccount");
    const fetch = mockFetch(
      jsonResponse({ required: false, hasAdmin: true, appVersion: "1.0" }),
      jsonResponse({ complete: true, token: "token", user: { id: "u1", email: "user@example.com", role: "user" } }),
    );
    renderWithQuery(<LoginPage />);
    await userEvent.type(await screen.findByLabelText("Email address"), "USER@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "secret-password");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/account"));
    expect(requestJSON(fetch.calls[1])).toEqual({ email: "user@example.com", password: "secret-password" });
  });
});

describe("login checkpoint", () => {
  it("supports switching to a recovery code", async () => {
    mockFetch(
      jsonResponse({ required: false, hasAdmin: true, appVersion: "1.0" }),
      jsonResponse({ complete: false, confirmationToken: "checkpoint" }),
      jsonResponse({ complete: true, token: "token", user: { id: "u1", email: "user@example.com", role: "user", useTotp: true } }),
    );
    renderWithQuery(<LoginPage />);
    await userEvent.type(await screen.findByLabelText("Email address"), "user@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "secret-password");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await userEvent.click(await screen.findByRole("button", { name: "Use a recovery code instead" }));
    expect(screen.getByLabelText("Recovery code")).toHaveAttribute("autocomplete", "one-time-code");
    await userEvent.type(screen.getByLabelText("Recovery code"), "backup-code");
    await userEvent.click(screen.getByRole("button", { name: "Verify and continue" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/servers"));
  });
});

describe("password recovery", () => {
  it("uses a privacy-preserving success state after mail is accepted", async () => {
    mockFetch(jsonResponse({ status: "sent" }));
    renderWithQuery(<ForgotPasswordPage />);
    await userEvent.type(screen.getByLabelText("Email address"), "person@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(await screen.findByText("Check your inbox")).toBeInTheDocument();
    expect(screen.getByText(/response is the same whether or not/i)).toBeInTheDocument();
  });

  it("rejects reset links without both a token and valid email", () => {
    search = new URLSearchParams("token=abc");
    renderWithQuery(<ResetPasswordPage />);
    expect(screen.getByRole("alert")).toHaveTextContent("Invalid reset link");
    expect(screen.getByRole("link", { name: "Request a new link" })).toHaveAttribute("href", "/forgot-password");
  });
});
