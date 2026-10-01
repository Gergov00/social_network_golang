let tail: Promise<unknown> = Promise.resolve();

// Browser Set-Cookie precedes JS callbacks. Await the entire old response
// before dispatching the next operation that changes the refresh cookie.
export function queueAuth<T>(request: () => Promise<T>): Promise<T> {
  const result = tail.then(request);
  tail = result.catch(() => {});
  return result;
}
