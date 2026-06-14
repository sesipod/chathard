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
const IMAGE_KEY = `${STORAGE_PREFIX}autoShowImages`;

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

// ── Auto-show images ──

const initialAutoImages = readFromStorage(IMAGE_KEY, 'true') === 'true';
export const autoShowImages = writable(initialAutoImages);

autoShowImages.subscribe((value) => {
  writeToStorage(IMAGE_KEY, String(value));
});

export const settings = {
  theme,
  defaultRetention,
  autoShowImages,
  toggleTheme,
  setTheme,
  setDefaultRetention,
};
