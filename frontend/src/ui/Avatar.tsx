type Props = {
  id: string;
  name: string;
  handle: string;
  size?: number;
  online?: boolean;
  square?: boolean;
};

// Палитра градиентов из прототипа: там цвет лежал в моках (u.c), бэкенд его
// не отдаёт. Выбираем по хешу id, чтобы у одного человека аватар не менял
// цвет между лентой и профилем.
const GRADIENTS = [
  'linear-gradient(135deg,#13c4bd,#0f9a95)',
  'linear-gradient(135deg,#6a5cf0,#4b3fd0)',
  'linear-gradient(135deg,#2a86d6,#1d63a6)',
  'linear-gradient(135deg,#2aa77c,#1d7d5c)',
  'linear-gradient(135deg,#e0556b,#b93d51)',
  'linear-gradient(135deg,#f0a martin,#d98324)'.replace(' martin', '35c'),
];

function hash(value: string): number {
  let acc = 0;
  for (let i = 0; i < value.length; i += 1) {
    acc = (acc * 31 + value.charCodeAt(i)) >>> 0;
  }
  return acc;
}

// Инициалы: из имени, а если его нет — из handle. Пустое имя у нас штатное
// состояние: user-service заводит профиль с Name="" и стаб-хендлом.
export function initials(name: string, handle: string): string {
  const source = name.trim() || handle.trim();
  if (!source) return '?';

  const words = source.split(/[\s_.-]+/).filter(Boolean);
  const letters = words.length >= 2 ? words[0][0] + words[1][0] : source.slice(0, 2);
  return letters.toUpperCase();
}

export function Avatar({ id, name, handle, size = 44, online = false, square = false }: Props) {
  const className = ['ava', online ? 'ava-on' : '', square ? 'sq' : ''].filter(Boolean).join(' ');

  return (
    <div
      className={className}
      aria-hidden="true"
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.4),
        background: GRADIENTS[hash(id) % GRADIENTS.length],
      }}
    >
      {initials(name, handle)}
    </div>
  );
}
