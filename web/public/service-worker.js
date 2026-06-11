const CACHE_NAME = 'tailchat-v1';
const ASSETS = [
  '/',
  '/index.html',
];

// ── Install: cache app shell ──
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(ASSETS);
    })
  );
  self.skipWaiting();
});

// ── Activate: clean old caches ──
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys.map((key) => {
          if (key !== CACHE_NAME) return caches.delete(key);
        })
      )
    )
  );
  self.clients.claim();
});

// ── Fetch: serve from cache, fall back to network ──
self.addEventListener('fetch', (event) => {
  // Only cache same-origin GET requests for app shell assets
  if (event.request.method !== 'GET') return;
  if (!event.request.url.startsWith(self.location.origin)) return;

  event.respondWith(
    caches.match(event.request).then((cached) => {
      return cached || fetch(event.request);
    })
  );
});

// ── Background Sync: retry failed message sends ──
self.addEventListener('sync', (event) => {
  if (event.tag === 'send-message') {
    event.waitUntil(retrySendMessage());
  }
});

async function retrySendMessage() {
  // Read pending messages from IndexedDB and attempt to POST them.
  // This is a stub — the full implementation will be wired in Phase 4.
  const cache = await caches.open('tailchat-pending');
  const requests = await cache.keys();
  for (const req of requests) {
    try {
      await fetch(req.clone());
      await cache.delete(req);
    } catch {
      // Keep in cache for next retry
    }
  }
}

// ── Notification click: focus or open PWA ──
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const urlToOpen = '/';

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      // Focus existing PWA window if available
      for (const client of clients) {
        if (client.url === urlToOpen || client.url.startsWith(self.location.origin)) {
          return client.focus();
        }
      }
      // Otherwise open new window
      return self.clients.openWindow(urlToOpen);
    })
  );
});

// ── Push notification handler ──
self.addEventListener('push', (event) => {
  let data;
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = {};
  }

  const handle = data.handle || 'someone';
  const title = `New message from @${handle}`;

  const options = {
    body: '', // Zero content preview — never leak message content
    icon: '/icons/icon-192.png',
    badge: '/icons/icon-192.png',
    vibrate: [200, 100, 200],
    tag: 'tailchat-message',
    data: {
      url: data.url || '/',
    },
  };

  event.waitUntil(self.registration.showNotification(title, options));
});
