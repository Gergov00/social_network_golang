import { useState } from 'react';
import { Link, Navigate, useNavigate } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { LogoMark } from '../../ui/Icons';
import { Spinner } from '../../ui/Spinner';

type Props = { mode: 'login' | 'register' };

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function messageFor(err: unknown, mode: Props['mode']): string {
  if (err instanceof ApiError) {
    if (err.status === 409) return 'Этот e-mail уже занят';
    if (err.status === 401) return 'Неверный e-mail или пароль';
    if (err.status >= 500) return 'Сервер недоступен, попробуйте позже';
  }
  return mode === 'login' ? 'Не удалось войти' : 'Не удалось создать аккаунт';
}

export function AuthPage({ mode }: Props) {
  const { status, login, register } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (status === 'loading') return <Spinner />;
  if (status === 'authed') return <Navigate to="/" replace />;

  const title = mode === 'login' ? 'Войти' : 'Создать аккаунт';

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setFormError(null);

    const errors: typeof fieldErrors = {};
    if (!EMAIL_RE.test(email)) errors.email = 'Проверьте e-mail';
    // 8 символов, а не 6 из прототипа: столько требует auth-service.
    if (password.length < 8) errors.password = 'Минимум 8 символов';
    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;

    setBusy(true);
    try {
      if (mode === 'login') await login(email, password);
      else await register(email, password);
      navigate('/', { replace: true });
    } catch (err) {
      setFormError(messageFor(err, mode));
    } finally {
      setBusy(false);
    }
  };

  // Разметка — по макету: .auth > .auth-card, поля .field > label + .inp,
  // ошибка поля .hint, ссылка внизу .auth-alt.
  return (
    <div className="auth">
      <div className="auth-card">
        <div className="auth-logo">
          <LogoMark size={34} />
        </div>
        <h1 className="h-title" style={{ textAlign: 'center' }}>{title}</h1>
        <p className="h-sub" style={{ textAlign: 'center', marginBottom: 22 }}>
          ahishka — социальная сеть
        </p>

        <form className="card" onSubmit={submit} noValidate style={{ padding: 'var(--card-pad)' }}>
          <div className="field">
            <label htmlFor="auth-email">E-mail</label>
            <input
              id="auth-email"
              className={`inp${fieldErrors.email ? ' err' : ''}`}
              type="email"
              value={email}
              autoComplete="email"
              onChange={(e) => setEmail(e.target.value)}
            />
            {fieldErrors.email && <div className="hint">{fieldErrors.email}</div>}
          </div>

          <div className="field">
            <label htmlFor="auth-password">Пароль</label>
            <input
              id="auth-password"
              className={`inp${fieldErrors.password ? ' err' : ''}`}
              type="password"
              value={password}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              onChange={(e) => setPassword(e.target.value)}
            />
            {fieldErrors.password && <div className="hint">{fieldErrors.password}</div>}
          </div>

          {formError && (
            <div className="errorbox" role="alert" style={{ marginBottom: 14 }}>
              {formError}
            </div>
          )}

          <button className="btn btn-primary btn-block" type="submit" disabled={busy}>
            {busy ? 'Отправляем…' : title}
          </button>
        </form>

        <div className="auth-alt">
          {mode === 'login' ? (
            <>Нет аккаунта? <Link to="/register">Создать</Link></>
          ) : (
            <>Уже есть аккаунт? <Link to="/login">Войти</Link></>
          )}
        </div>
      </div>
    </div>
  );
}
