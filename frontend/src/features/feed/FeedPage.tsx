// Заглушка Task 4 (замена в Task 5): в брифе .composer-fake был текстовым
// div'ом, но тест ждёт `findByPlaceholderText` — div его не пройдёт ни при
// каком реальном composer'е. Меняем на input с тем же текстом в placeholder:
// стаб остаётся одной строкой, а тест стережёт реальный доступный признак.
export function FeedPage() {
  return (
    <div className="center-col">
      <input className="composer-fake" placeholder="Поделитесь мыслью…" readOnly />
    </div>
  );
}
