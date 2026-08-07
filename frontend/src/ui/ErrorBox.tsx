export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="errorbox" role="alert">
      <div>{message}</div>
      {onRetry && (
        <button className="btn btn-ghost btn-sm" style={{ marginTop: 12 }} onClick={onRetry}>
          Повторить
        </button>
      )}
    </div>
  );
}
