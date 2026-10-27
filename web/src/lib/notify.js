/**
 * Alerts for incoming messages while the app is open (including in a
 * background tab). Nothing here leaves the browser, and nothing is sent to any
 * push service: it only works while the page is loaded.
 *
 *  - a short beep (generated, no audio file)
 *  - an optional browser notification — text is just "New message" unless the
 *    user opts in to showing the sender's name; message content is never shown
 *  - an unread count in the tab title
 */
import { get } from 'svelte/store';
import { notifyDesktop, notifyShowSender, notifySound } from './stores/settings.js';

const BASE_TITLE = 'TailChat';
let audio = null;
let unread = 0;

function beep() {
  try {
    audio ||= new (window.AudioContext || window.webkitAudioContext)();
    if (audio.state === 'suspended') audio.resume();
    const now = audio.currentTime;
    const osc = audio.createOscillator();
    const gain = audio.createGain();
    osc.type = 'sine';
    osc.frequency.setValueAtTime(880, now);
    osc.frequency.setValueAtTime(660, now + 0.09);
    gain.gain.setValueAtTime(0.0001, now);
    gain.gain.exponentialRampToValueAtTime(0.15, now + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.25);
    osc.connect(gain).connect(audio.destination);
    osc.start(now);
    osc.stop(now + 0.26);
  } catch {
    /* audio blocked or unavailable: not worth failing over */
  }
}

function setTitle() {
  document.title = unread > 0 ? `(${unread}) ${BASE_TITLE}` : BASE_TITLE;
}

/** Reset the unread count when the user comes back to the tab. */
export function watchVisibility() {
  const clear = () => {
    if (!document.hidden) {
      unread = 0;
      setTitle();
    }
  };
  document.addEventListener('visibilitychange', clear);
  window.addEventListener('focus', clear);
  return () => {
    document.removeEventListener('visibilitychange', clear);
    window.removeEventListener('focus', clear);
  };
}

/** Ask the browser for permission. Must be called from a click. */
export async function enableDesktopNotifications() {
  if (!('Notification' in window)) return false;
  const result = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission();
  return result === 'granted';
}

/**
 * Call when a message from someone else arrives.
 * @param {{ viewing: boolean, senderHandle?: string, onClick?: () => void }} opts
 *   viewing — the user is looking at that very conversation right now
 */
export function notifyIncoming({ viewing, senderHandle, onClick }) {
  if (viewing) return; // already on screen: no need to interrupt
  if (get(notifySound)) beep();

  if (document.hidden) {
    unread++;
    setTitle();
  }

  if (get(notifyDesktop) && 'Notification' in window && Notification.permission === 'granted') {
    try {
      const body = get(notifyShowSender) && senderHandle ? `New message from @${senderHandle}` : 'New message';
      const n = new Notification(BASE_TITLE, { body, tag: 'tailchat-message', renotify: true, silent: true, icon: '/icons/icon.svg' });
      n.onclick = () => {
        window.focus();
        onClick?.();
        n.close();
      };
    } catch {
      /* some mobile browsers only allow notifications via a service worker */
    }
  }
}
