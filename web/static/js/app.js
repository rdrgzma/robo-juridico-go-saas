(() => {
  const root = document.documentElement;
  try {
    const saved = localStorage.getItem('robo_juridico_theme');
    root.dataset.theme = saved === 'light' || saved === 'dark' ? saved : (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
  } catch (_) { /* The workspace remains usable without browser storage. */ }
  document.getElementById('theme-toggle')?.addEventListener('click', () => {
    root.dataset.theme = root.dataset.theme === 'light' ? 'dark' : 'light';
    try { localStorage.setItem('robo_juridico_theme', root.dataset.theme); } catch (_) {}
  });
})();
