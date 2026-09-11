"use client";

import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { fetchCurrentUser } from "./auth";
import { queryKeys } from "./query-keys";
import type { ApiUser } from "./types";

/**
 * Single owner of the current-user query. Every surface that needs the
 * session user goes through here so the key, staleTime and retry policy
 * cannot drift apart per call site.
 */
export function useCurrentUser(): UseQueryResult<ApiUser | null> {
  return useQuery({
    queryKey: queryKeys.session.currentUser(),
    queryFn: fetchCurrentUser,
    staleTime: 30_000,
    retry: 1,
  });
}
