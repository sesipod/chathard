/**
 * Settings store — app preferences persisted to localStorage.
 *
 * Stores:
 *   theme             {'dark'|'light'|'system'}  color scheme preference
 *   defaultRetention  {'Never'|'1h'|'24h'|'7d'|'30d'|'90d'}  default auto-delete
 *
 * Actions:
 *   toggleTheme()        — cycle dark → light → system → dark
 *   setTheme(mode)       — explicitly set theme
 *   setDefaultRetention  — set default message retention
 */
import { writable } from 'svelte/store';

const STORAGE_PREFIX = 'tailchat-';
const THEME_KEY = `${STORAGE_PREFIX}theme`;
const RETENTION_KEY = `${STORAGE_PREFIX}retention`;

// ── Helpers ──

function readFromStorage(key, fallback) {
  try {
    const v = localStorage.getItem(key);
    return v !== null ? v : fallback;
  } catch {
    return fallback;
  }
}

function writeToStorage(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // localStorage may be unavailable (private browsing, quota)
  }
}

// ── Theme ──

const initialTheme = readFromStorage(THEME_KEY, 'dark');
export const theme = writable(initialTheme);

// Apply theme immediately on import
applyThemeToDocument(initialTheme);

// Subscribe to changes and persist
theme.subscribe((value) => {
  writeToStorage(THEME_KEY, value);
  applyThemeToDocument(value);
});

function applyThemeToDocument(mode) {
  if (typeof document === 'undefined') return;
  if (mode === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.body.classList.toggle('light', !prefersDark);
    return;
  }
  document.body.classList.toggle('light', mode === 'light');
}

function toggleTheme() {
  theme.update((t) => {
    if (t === 'dark') return 'light';
    if (t === 'light') return 'system';
    return 'dark';
  });
}

function setTheme(mode) {
  if (['dark', 'light', 'system'].includes(mode)) {
    theme.set(mode);
  }
}

// ── Default Retention ──

const initialRetention = readFromStorage(RETENTION_KEY, 'Never');
export const defaultRetention = writable(initialRetention);

defaultRetention.subscribe((value) => {
  writeToStorage(RETENTION_KEY, value);
});

function setDefaultRetention(value) {
  if (['Never', '1h', '24h', '7d', '30d', '90d'].includes(value)) {
    defaultRetention.set(value);
  }
}

// ── Per-conversation auto-show images ──

const IMAGE_PREFIX = `${STORAGE_PREFIX}showImages-`;

/** Reactive counter — increments whenever any per-conversation setting changes */
export const imageSettingsVersion = writable(0);

export function getAutoShowImages(convId) {
  try { return localStorage.getItem(IMAGE_PREFIX + convId) !== 'false'; } catch { return true; }
}

export function setAutoShowImages(convId, value) {
  try {
    localStorage.setItem(IMAGE_PREFIX + convId, String(value));
    imageSettingsVersion.update(v => v + 1); // notify subscribers
  } catch {}
}

// ── Notifications (all local to this browser) ──
//
// Sound is on by default. Desktop notifications are OFF until the user turns
// them on (browsers require a click to grant permission). Notification text is
// generic by default — "New message" — because notification centres and lock
// screens can be read by other people and logged by the OS.

const NOTIFY_SOUND = `${STORAGE_PREFIX}notify-sound`;
const NOTIFY_DESKTOP = `${STORAGE_PREFIX}notify-desktop`;
const NOTIFY_SENDER = `${STORAGE_PREFIX}notify-sender`;

export const notifySound = writable(readFromStorage(NOTIFY_SOUND, 'true') === 'true');
export const notifyDesktop = writable(readFromStorage(NOTIFY_DESKTOP, 'false') === 'true');
export const notifyShowSender = writable(readFromStorage(NOTIFY_SENDER, 'false') === 'true');
notifySound.subscribe((v) => writeToStorage(NOTIFY_SOUND, String(v)));
notifyDesktop.subscribe((v) => writeToStorage(NOTIFY_DESKTOP, String(v)));
notifyShowSender.subscribe((v) => writeToStorage(NOTIFY_SENDER, String(v)));

export const settings = {
  notifySound,
  notifyDesktop,
  notifyShowSender,
  theme,
  defaultRetention,
  toggleTheme,
  setTheme,
  setDefaultRetention,
  getAutoShowImages,
  setAutoShowImages,
};
