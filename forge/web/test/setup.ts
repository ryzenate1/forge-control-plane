import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  try {
    window.localStorage?.clear();
  } catch {
    // jsdom without url may not expose localStorage
  }
});
