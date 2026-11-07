import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App.tsx'
import { ThemeBootstrap } from './theme/ThemeProvider.tsx'
import { ToastProvider } from './components/ui/Toast.tsx'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeBootstrap>
        <ToastProvider>
          <App />
        </ToastProvider>
      </ThemeBootstrap>
    </BrowserRouter>
  </StrictMode>,
)
