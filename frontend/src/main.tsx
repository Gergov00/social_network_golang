import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Providers } from './app/providers';
import { AppRouter } from './app/router';
// Порядок важен: app.css — дословная копия макета, tokens.css поверх него
// переопределяет акцент. Поменять местами значит потерять фирменный цвет.
import './styles/app.css';
import './styles/tokens.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Providers>
      <AppRouter />
    </Providers>
  </StrictMode>,
);
