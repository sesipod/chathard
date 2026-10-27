import { chromium } from 'playwright-core';
import fs from 'node:fs';
import assert from 'node:assert/strict';

// Browser end-to-end test. Needs: playwright-core, chromium, a running server (see run.sh).
const DIR = process.env.E2E_DIR || '/tmp/tc-e2e';
const BASE = 'http://127.0.0.1:3100';
const browser = await chromium.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'] });
const { DatabaseSync: DB2 } = await import('node:sqlite');
const peek = (sql) => { const d = new DB2(DIR + '/data/t.db', { readOnly: true }); d.exec('PRAGMA busy_timeout=5000'); try { return d.prepare(sql).all(); } finally { d.close(); } };
const results = [];
const secretsExtra = [];
const ok = (name) => { results.push(name); console.log('  ✔', name); };

async function newUser(handle) {
  const ctx = await browser.newContext({ baseURL: BASE, permissions: ['notifications'] });
  // record every Notification and every beep so the test can assert on them
  await ctx.addInitScript(() => {
    window.__notes = []; window.__beeps = 0;
    const N = window.Notification;
    if (N) window.Notification = class extends N { constructor(t, o) { super(t, o); window.__notes.push({ title: t, body: o && o.body }); } };
    const AC = window.AudioContext;
    if (AC) { const co = AC.prototype.createOscillator; AC.prototype.createOscillator = function () { window.__beeps++; return co.apply(this, arguments); }; }
  });
  const page = await ctx.newPage();
  page.on('pageerror', (e) => console.log(`  [${handle}] pageerror:`, e.message));
  page.on('response', async (r) => { if (r.status() >= 400 && r.status() !== 401 && r.status() !== 404) console.log(`  [${handle}] HTTP ${r.status()} ${r.request().method()} ${new URL(r.url()).pathname} -> ${(await r.text().catch(() => '')).trim().slice(0, 80)}`); });
  page.on('console', (m) => { if (m.type() === 'error') console.log(`  [${handle}] console.error:`, m.text()); });
  return { ctx, page, handle };
}

async function register(u) {
  const { page, handle } = u;
  await page.goto('/');
  await page.getByRole('button', { name: 'Create Account' }).first().click();
  await page.locator('#reg-handle').fill(handle);
  await page.getByText('✓ Available').waitFor();
  await page.locator('form button[type=submit]').click();
  await page.locator('.key-box').waitFor();
  u.key = (await page.locator('.key-box').innerText()).trim();
  assert.match(u.key, /^([A-Z2-9]{5}-){10}[A-Z2-9]{5}$/);
  await page.locator('.check input[type=checkbox]').first().check();
  await page.getByRole('button', { name: 'Start Chatting' }).click();
  await page.getByLabel('New conversation').waitFor();
}

async function signIn(u, key) {
  const { page, handle } = u;
  await page.goto('/');
  await page.locator('#login-handle').fill(handle);
  await page.locator('#login-key').fill(key);
  await page.locator('form button[type=submit]').click();
  await page.getByLabel('New conversation').waitFor();
}

async function openChatWith(u, other) {
  const { page } = u;
  await page.getByLabel('New conversation').click();
  await page.locator('.new-menu-item', { hasText: 'New Chat' }).click();
  await page.getByPlaceholder('Search by handle (min 3 chars)...').fill(other);
  await page.getByLabel(`Start chat with ${other}`).click();
}

async function send(u, text) {
  const { page } = u;
  await page.getByLabel('Message text').fill(text);
  await page.getByLabel('Send message').click();
}

const bubble = (page, text) => page.locator('.bubble-text', { hasText: text });

console.log('\n== Register three users ==');
const alice = await newUser('alice'), bob = await newUser('bobby'), carol = await newUser('carol');
await register(alice); ok('alice registered; account key shown');
await register(bob); ok('bobby registered');
await register(carol); ok('carol registered');

console.log('\n== 1:1 text ==');
const SECRET = 'the eagle lands at midnight (e2e-secret-4242)';
await openChatWith(alice, 'bobby');
await send(alice, SECRET);
await bubble(alice.page, SECRET).waitFor();
ok('alice sees her own message');
await bob.page.locator('.chat-name', { hasText: 'alice' }).first().waitFor({ timeout: 15000 });
await bob.page.locator('.chat-name', { hasText: 'alice' }).first().click();
await bubble(bob.page, SECRET).waitFor({ timeout: 15000 });
ok('bobby received it live over WebSocket and decrypted it');
await send(bob, 'got it — reply-secret-9001');
await bubble(alice.page, 'reply-secret-9001').waitFor({ timeout: 15000 });
ok('alice received bobby\'s reply');

console.log('\n== Image + file ==');
await alice.page.locator('input[type=file]').setInputFiles(DIR + '/red.png');
await bob.page.locator('img.inline-image').waitFor({ timeout: 20000 });
const natural = await bob.page.locator('img.inline-image').evaluate((i) => i.naturalWidth);
assert.equal(natural, 1);
ok('bobby sees the decrypted image inline');
await alice.page.locator('input[type=file]').setInputFiles(DIR + '/secret-plans.txt');
await bob.page.getByRole('button', { name: /secret-plans\.txt/ }).waitFor({ timeout: 20000 });
const [dl] = await Promise.all([
  bob.page.waitForEvent('download'),
  bob.page.getByRole('button', { name: /secret-plans\.txt/ }).click(),
]);
assert.equal(fs.readFileSync(await dl.path(), 'utf8'), 'TOP-SECRET-FILE-CONTENT-7731\n');
assert.equal(dl.suggestedFilename(), 'secret-plans.txt');
ok('bobby downloaded the file; contents and filename match');

console.log('\n== Logout, then read history again (same device) ==');
await alice.page.getByLabel('Settings').click();
await alice.page.getByRole('button', { name: 'Log out' }).first().click();
await alice.page.locator('.dlg').getByRole('button', { name: 'Log out' }).click();
await alice.page.locator('#login-handle').waitFor();
await signIn(alice, alice.key);
await alice.page.locator('.chat-name', { hasText: 'bobby' }).first().click();
await bubble(alice.page, SECRET).waitFor({ timeout: 15000 });
await bubble(alice.page, 'reply-secret-9001').waitFor();
ok('after logout + sign-in, alice reads both what she SENT and what she received');

console.log('\n== Brand-new device, only the account key ==');
const aliceTablet = await newUser('alice');
await signIn(aliceTablet, alice.key);
await aliceTablet.page.locator('.chat-name', { hasText: 'bobby' }).first().click();
await bubble(aliceTablet.page, SECRET).waitFor({ timeout: 15000 });
await bubble(aliceTablet.page, 'reply-secret-9001').waitFor();
await aliceTablet.page.locator('img.inline-image').waitFor({ timeout: 20000 });
ok('fresh browser (tablet): full history incl. her own messages and the image decrypts');

console.log('\n== Wrong key is refused ==');
const evil = await newUser('alice');
await evil.page.goto('/');
await evil.page.locator('#login-handle').fill('alice');
await evil.page.locator('#login-key').fill(bob.key);
await evil.page.locator('form button[type=submit]').click();
await evil.page.locator('.error-banner').waitFor();
assert.match(await evil.page.locator('.error-banner').innerText(), /doesn't belong/);
ok('bobby\'s key cannot sign in as alice');

console.log('\n== Group ==');
await alice.page.getByLabel('New conversation').click();
await alice.page.locator('.new-menu-item', { hasText: 'New Group' }).click();
await alice.page.getByLabel('Group name').fill('Top Secret Crew');
await alice.page.getByRole('button', { name: 'Next' }).click();
for (const h of ['bobby', 'carol']) {
  await alice.page.getByPlaceholder('Search by handle (min 3 chars)...').fill(h);
  await alice.page.getByLabel(`Add ${h}`).click();
}
await alice.page.getByRole('button', { name: /Next \(2\)/ }).click();
await alice.page.getByRole('button', { name: 'Create Group' }).click();
await alice.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().click();
const GSEC = 'group-secret-launch-5566';
await send(alice, GSEC);
await bubble(alice.page, GSEC).waitFor();

console.log('\n== Invitations: nothing is readable until you accept ==');
const GID = peek('select id from groups')[0].id;
assert.deepEqual(peek("select status from group_members where user_id in (select id from users where handle in ('bobby','carol'))").map((r) => r.status), ['invited', 'invited']);
for (const u of [bob, carol]) {
  await u.page.reload();
  await u.page.locator('.invite', { hasText: '@alice invited you' }).waitFor({ timeout: 15000 });
  assert.equal(await u.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).count(), 0, 'group must not be listed before accepting');
  const codes = await u.page.evaluate(async (gid) => {
    const h = { Authorization: 'Bearer ' + sessionStorage.getItem('tailchat-token') };
    return Promise.all([`/api/groups/${gid}/messages`, `/api/groups/${gid}/keys`, `/api/messages?group_id=${gid}`].map(async (p) => (await fetch(p, { headers: h })).status));
  }, GID);
  assert.deepEqual(codes, [404, 404, 404]);
}
ok('bobby and carol see an invitation only; the server refuses them messages and keys');

// carol declines a second group: the key addressed to her is deleted
await alice.page.getByLabel('New conversation').click();
await alice.page.locator('.new-menu-item', { hasText: 'New Group' }).click();
await alice.page.getByLabel('Group name').fill('Unwanted Group');
await alice.page.getByRole('button', { name: 'Next' }).click();
await alice.page.getByPlaceholder('Search by handle (min 3 chars)...').fill('carol');
await alice.page.getByLabel('Add carol').click();
await alice.page.getByRole('button', { name: /Next \(1\)/ }).click();
await alice.page.getByRole('button', { name: 'Create Group' }).click();
await carol.page.locator('.invite').nth(1).waitFor({ timeout: 15000 });
const unwantedId = peek("select id from groups where id != '" + GID + "'")[0].id;
// invitations are listed newest first: the second group is on top
await carol.page.locator('.invite').first().locator('button.decline').click();
for (let i = 0; i < 20 && peek("select count(*) c from group_members where group_id='" + unwantedId + "' and status='invited'")[0].c > 0; i++) await new Promise((r) => setTimeout(r, 250));
assert.equal(peek("select count(*) c from group_members where group_id='" + unwantedId + "' and user_id=(select id from users where handle='carol')")[0].c, 0);
assert.equal(peek("select count(*) c from group_key_envelopes where group_id='" + unwantedId + "' and user_id=(select id from users where handle='carol')")[0].c, 0);
ok('carol declined: membership AND the key envelope addressed to her are deleted');

// accept the real group
for (const u of [bob, carol]) {
  await u.page.locator('.invite', { hasText: '@alice invited you' }).locator('button.accept').first().click();
  await u.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().waitFor({ timeout: 15000 });
  await u.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().click();
  await bubble(u.page, GSEC).waitFor({ timeout: 15000 });
}
ok('group: name and message decrypt for both members');
await send(carol, 'carol-in-group-7788');
await bubble(alice.page, 'carol-in-group-7788').waitFor({ timeout: 15000 });
ok('group: carol -> alice');

console.log('\n== Member leaves: admin rotates the group key, leaver is locked out ==');
await carol.page.reload();
await carol.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().click();
await carol.page.getByLabel('More options').click();
await carol.page.getByRole('menuitem', { name: 'Leave group' }).click();
for (let i = 0; i < 40 && peek("select epoch from groups")[0].epoch < 1; i++) await new Promise((r) => setTimeout(r, 250));
assert.equal(peek("select epoch from groups")[0].epoch, 1, "admin's browser should have rotated the key");
assert.deepEqual(peek("select count(*) c from group_members where user_id=(select id from users where handle='carol')")[0].c, 0);
assert.equal(peek("select count(*) c from group_key_envelopes where epoch=1")[0].c, 2, 'new key only sealed to the 2 remaining members');
ok('carol left; alice\'s browser rotated the key to epoch 1 for alice+bobby only');
const POST = 'after-leaving-secret-3141';
await send(alice, POST);
await bubble(alice.page, POST).waitFor();
await bob.page.reload();
await bob.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().click();
await bubble(bob.page, POST).waitFor({ timeout: 15000 });
await bubble(bob.page, GSEC).waitFor();
ok('bobby reads the new epoch AND the old history');
await carol.page.reload();
await carol.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).count().then((n) => assert.equal(n, 0));
const carolCanFetch = await carol.page.evaluate(async (gid) => {
  const r = await fetch(`/api/groups/${gid}/messages`, { headers: { Authorization: 'Bearer ' + sessionStorage.getItem('tailchat-token') } });
  return r.status;
}, peek('select id from groups')[0].id);
assert.equal(carolCanFetch, 404);
ok('carol no longer sees the group and the server refuses her the ciphertext');
secretsExtra.push(POST);

console.log('\n== Alerts while the app is open ==');
const notes = () => bob.page.evaluate(() => ({ n: window.__notes.slice(), b: window.__beeps }));
await bob.page.getByLabel('Settings').click();
await bob.page.locator('label.toggle').filter({ has: bob.page.getByLabel('Desktop notifications') }).click();
assert.equal(await bob.page.getByLabel('Desktop notifications').isChecked(), true);
await bob.page.locator('.hdr .back').click();
// bobby is looking at the group; alice messages him privately -> alert
await alice.page.locator('.chat-name', { hasText: 'bobby' }).first().click();
const before = await notes();
await send(alice, 'notify-test-8080');
await bob.page.waitForFunction((n) => window.__notes.length > n, before.n.length, { timeout: 15000 });
let after = await notes();
const last = after.n[after.n.length - 1];
assert.deepEqual(last, { title: 'TailChat', body: 'New message' });
assert.ok(after.b > before.b, 'a beep should play');
assert.ok(!JSON.stringify(after.n).includes('notify-test') && !JSON.stringify(after.n).includes('alice'), 'no content or name by default');
ok('background chat: beep + notification that says only "New message" (no sender, no content)');

await bob.page.getByLabel('Settings').click();
await bob.page.locator('label.toggle').filter({ has: bob.page.getByLabel('Show sender name in notifications') }).click();
await bob.page.locator('.hdr .back').click();
await send(alice, 'notify-test-sender-9090');
await bob.page.waitForFunction((n) => window.__notes.length > n, after.n.length, { timeout: 15000 });
after = await notes();
assert.equal(after.n[after.n.length - 1].body, 'New message from @alice');
assert.ok(!JSON.stringify(after.n).includes('notify-test'), 'message text must never be in a notification');
ok('with the opt-in, the sender\'s name is shown; message text never is');

// no alert when bobby is already looking at that conversation, or for his own messages
await bob.page.locator('.chat-name', { hasText: 'alice' }).first().click();
await bob.page.waitForTimeout(500);
const quietBefore = await notes();
await send(alice, 'quiet-while-viewing-1');
await bubble(bob.page, 'quiet-while-viewing-1').waitFor({ timeout: 15000 });
await send(bob, 'bobs-own-message-2');
await bubble(alice.page, 'bobs-own-message-2').waitFor({ timeout: 15000 });
await bob.page.waitForTimeout(1000);
const quietAfter = await notes();
assert.equal(quietAfter.n.length, quietBefore.n.length);
assert.equal(quietAfter.b, quietBefore.b);
ok('no alert for the chat you are viewing or for your own messages');
secretsExtra.push('notify-test-8080', 'notify-test-sender-9090', 'quiet-while-viewing-1', 'bobs-own-message-2');

console.log('\n== Hardening checks (raw HTTP / WebSocket) ==');
const token = await alice.page.evaluate(() => sessionStorage.getItem('tailchat-token'));
const get = (path, init = {}) => fetch(BASE + path, init);
{
  const r = await get('/');
  const csp = r.headers.get('content-security-policy') || '';
  assert.match(csp, /script-src 'self'/); assert.match(csp, /default-src 'none'/); assert.match(csp, /frame-ancestors 'none'/);
  assert.ok(!/unsafe-inline[^;]*script|script-src[^;]*unsafe/.test(csp), 'scripts must not be allowed inline');
  assert.equal((await get('/api/health')).headers.get('cache-control'), 'no-store');
  ok('CSP forbids inline/third-party scripts; API responses are no-store');
}
assert.equal((await get('/api/users/search?handle=ali')).status, 401);
assert.equal((await get('/api/users/search?handle=ali', { headers: { Authorization: 'Bearer ' + token } })).status, 200);
assert.deepEqual(await (await get('/api/users/search?handle=%25%25%25', { headers: { Authorization: 'Bearer ' + token } })).json(), []);
ok('user search needs a login, and wildcards cannot list everyone');
assert.equal((await (await get('/api/handle-available?handle=ALICE')).json()).available, false);
assert.equal((await (await get('/api/handle-available?handle=someone-new')).json()).available, true);
ok('handle check is case-insensitive (no "Alice" vs "alice" lookalikes)');
{
  const real = await (await get('/api/auth/challenge', { method: 'POST', body: '{"handle":"alice"}' })).status;
  const fake = await (await get('/api/auth/challenge', { method: 'POST', body: '{"handle":"does-not-exist"}' })).status;
  assert.equal(real, 200); assert.equal(fake, 200);
  ok('sign-in challenge looks identical for real and unknown handles');
}
assert.equal((await get('/api/me?token=' + token)).status, 401);
ok('session token in a URL is rejected (header only)');
{
  const { request } = await import('node:http');
  const status = await new Promise((res, rej) => {
    const q = request({ host: '127.0.0.1', port: 3100, path: '/api/health', headers: { Host: 'evil.example' } }, (r) => { r.resume(); res(r.statusCode); });
    q.on('error', rej); q.end();
  });
  assert.equal(status, 404);
  ok('DNS-rebinding style Host header is refused');
}
{
  const body = await (await get('/assets/')).text();
  assert.match(body, /<div id="app">/); assert.ok(!/<a href="index-/.test(body));
  ok('static directories are not listable');
}
{
  const out = await alice.page.evaluate(async (tok) => {
    const open = (url) => new Promise((res) => { const w = new WebSocket(url); w.onopen = () => res('open'); w.onerror = () => res('refused'); w.onclose = () => res('refused'); });
    const base = `ws://${location.host}/ws`;
    const viaToken = await open(`${base}?token=${tok}`);
    const t = (await (await fetch('/api/ws-ticket', { method: 'POST', headers: { Authorization: 'Bearer ' + tok } })).json()).ticket;
    const first = await open(`${base}?ticket=${t}`);
    const reuse = await open(`${base}?ticket=${t}`);
    const none = await open(base);
    const noAuth = (await fetch('/api/ws-ticket', { method: 'POST' })).status;
    return { viaToken, first, reuse, none, noAuth };
  }, token);
  assert.deepEqual(out, { viaToken: 'refused', first: 'open', reuse: 'refused', none: 'refused', noAuth: 401 });
  ok('WebSocket: session token refused, one-time ticket works exactly once');
}

console.log('\n== Malicious server swaps a contact\'s key ==');
const { DatabaseSync } = await import('node:sqlite');
const dbh = new DatabaseSync(DIR + '/data/t.db'); dbh.exec('PRAGMA busy_timeout=5000');
const carolRow = dbh.prepare("select public_key_ed25519 e, public_key_x25519 x from users where handle='carol'").get();
// an attacker with database access is not bound by the app's unique-key rule: free carol's keys, give them to bobby
dbh.prepare("update users set public_key_ed25519=randomblob(32), public_key_x25519=randomblob(32) where handle='carol'").run();
dbh.prepare("update users set public_key_ed25519=?, public_key_x25519=? where handle='bobby'").run(carolRow.e, carolRow.x);
dbh.close();
await alice.page.reload();
await alice.page.locator('.chat-name', { hasText: 'bobby' }).first().click();
await alice.page.locator('.security-alert').waitFor({ timeout: 15000 });
assert.match(await alice.page.locator('.security-alert').innerText(), /security key for bobby changed/);
await send(alice, 'this must never be encrypted to the swapped key');
await alice.page.locator('.toast', { hasText: 'changed' }).waitFor({ timeout: 10000 });
ok('alice is warned and her message is BLOCKED instead of being encrypted to the attacker\'s key');

console.log('\n== Server-side view ==');
fs.writeFileSync(DIR + '/secrets.json', JSON.stringify({
  strings: [...secretsExtra, SECRET, 'reply-secret-9001', GSEC, 'carol-in-group-7788', 'Top Secret Crew', 'TOP-SECRET-FILE-CONTENT-7731', 'secret-plans', 'red.png', alice.key, bob.key, carol.key,
            ...[alice.key, bob.key, carol.key].map((k) => k.replaceAll('-', ''))],
}));
await browser.close();
console.log(`\n${results.length} browser checks passed`);
