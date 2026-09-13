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
  const valueRef = useRef<T>(initial);
  valueRef.current = value;

  const update = useCallback(
    async (next: T) => {
      const prev = valueRef.current;
      previousRef.current = prev;
      const optimistic = options.onMutate(prev);
      setValue(optimistic);
      valueRef.current = optimistic;
      setPending(true);
      setRollbackError(null);

      try {
        const result = await mutationFn(next);
        const resolved = result ?? options.onMutate(prev);
        setValue(resolved);
        valueRef.current = resolved;
      } catch (error) {
        setValue(prev);
        valueRef.current = prev;
        const message = error instanceof Error ? error.message : "Update failed";
        setRollbackError(message);
        options.onError?.(error instanceof Error ? error : new Error(message), prev);
      } finally {
        setPending(false);
        options.onSettled?.();
      }
    },
    [mutationFn, options],
  );

  return { value, pending, rollbackError, update, setValue };
}
