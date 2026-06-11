<script>
  import Router, { wrap } from 'svelte-spa-router';
  import Login from '../views/Login.svelte';
  import Main from '../views/Main.svelte';

  /** Hash-based routes */
  const routes = {
    '/login': wrap({ component: Login }),
    '/chat': wrap({ component: Main }),
    '/chat/:id': wrap({ component: Main }),
    '*': Login,
  };

  // ── Theme provider ──
  const STORAGE_KEY = 'tailchat-theme';

  /** Apply the given theme ('dark'|'light') to document.body. */
  function applyTheme(theme) {
    document.body.classList.toggle('light', theme === 'light');
  }

  /** Load persisted theme or default to dark. */
  function loadTheme() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === 'light' || saved === 'dark') return saved;
    } catch { /* localStorage unavailable */ }
    return 'dark';
  }

  /** Persist and apply theme. Exposed globally so Settings can toggle. */
  window.__setTheme = (theme) => {
    try { localStorage.setItem(STORAGE_KEY, theme); } catch { /* noop */ }
    applyTheme(theme);
  };

  // Apply initial theme
  applyTheme(loadTheme());
</script>

<Router {routes} />
