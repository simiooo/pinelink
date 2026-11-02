if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('pinelink-sw.js').catch(() => {});
  });
}
