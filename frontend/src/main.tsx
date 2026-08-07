import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
// Порядок важен: app.css — дословная копия макета, tokens.css поверх него
// переопределяет акцент. Поменять местами значит потерять фирменный цвет.
import './styles/app.css';
import './styles/tokens.css';
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
