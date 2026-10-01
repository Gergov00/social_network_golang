import type { QueryClient } from '@tanstack/react-query';

const locks = new WeakMap<QueryClient, Map<string, Promise<void>>>();

// Lock includes onMutate: queued writes must snapshot the settled predecessor.
export async function lockMutation(client: QueryClient, key: string): Promise<() => void> {
  let entries = locks.get(client);
  if (!entries) { entries = new Map(); locks.set(client, entries); }
  const previous = entries.get(key);
  let release!: () => void;
  const current = new Promise<void>((resolve) => { release = resolve; });
  entries.set(key, current);
  await previous;
  return () => {
    release();
    if (entries.get(key) === current) entries.delete(key);
  };
}
