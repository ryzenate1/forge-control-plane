"use client";

import { useCallback, useRef, useState } from "react";

type OptimisticOptions<T> = {
  onMutate: (current: T) => T;
  onError?: (error: Error, previous: T) => void;
  onSettled?: () => void;
};

export function useOptimisticUpdate<T>(
  initial: T,
  mutationFn: (value: T) => Promise<T>,
  options: OptimisticOptions<T>,
) {
  const [value, setValue] = useState<T>(initial);
  const [pending, setPending] = useState(false);
  const [rollbackError, setRollbackError] = useState<string | null>(null);
  const previousRef = useRef<T>(initial);

  const update = useCallback(
    async (next: T) => {
      const prev = value;
      previousRef.current = prev;
      const optimistic = options.onMutate(prev);
      setValue(optimistic);
      setPending(true);
      setRollbackError(null);

      try {
        const result = await mutationFn(next);
        setValue(result ?? options.onMutate(prev));
      } catch (error) {
        setValue(prev);
        const message = error instanceof Error ? error.message : "Update failed";
        setRollbackError(message);
        options.onError?.(error instanceof Error ? error : new Error(message), prev);
      } finally {
        setPending(false);
        options.onSettled?.();
      }
    },
    [value, mutationFn, options],
  );

  return { value, pending, rollbackError, update, setValue };
}
