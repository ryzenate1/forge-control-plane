import { describe, expect, it } from "vitest";

import {
  evaluatePassword,
  firstPasswordError,
  isPasswordValid,
} from "@/components/ui/password-requirements";

describe("password requirements (mirror of backend ValidatePassword)", () => {
  it("accepts a password meeting every rule", () => {
    expect(isPasswordValid("Correct-horse-9")).toBe(true);
    expect(firstPasswordError("Correct-horse-9")).toBeNull();
  });

  it("rejects short passwords first", () => {
    expect(isPasswordValid("Short-1!")).toBe(false);
    expect(firstPasswordError("Short-1!")).toBe("Password must be at least 12 characters.");
  });

  it("flags each missing character class in backend order", () => {
    expect(evaluatePassword("alllowercase-1!").upper).toBe(false);
    expect(firstPasswordError("alllowercase-1!")).toBe(
      "Password must contain at least one uppercase letter.",
    );
    expect(firstPasswordError("ALLUPPERCASE-1!")).toBe(
      "Password must contain at least one lowercase letter.",
    );
    expect(firstPasswordError("NoDigitsHere-AB")).toBe("Password must contain at least one digit.");
    expect(firstPasswordError("NoSpecial123AB")).toBe(
      "Password must contain at least one special character.",
    );
  });

  it("treats any non-alphanumeric as special, like the backend", () => {
    expect(evaluatePassword("TwelveChars-12").special).toBe(true);
    expect(evaluatePassword("Twelve Chars 1A").special).toBe(true);
  });

  it("reports an empty password as too short", () => {
    expect(isPasswordValid("")).toBe(false);
    expect(firstPasswordError("")).toBe("Password must be at least 12 characters.");
  });
});
