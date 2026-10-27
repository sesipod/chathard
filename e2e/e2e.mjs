import { chromium } from 'playwright-core';
import fs from 'node:fs';
import assert from 'node:assert/strict';

// Browser end-to-end test. Needs: playwright-core, chromium, a running server (see run.sh).
const DIR = process.env.E2E_DIR || '/tmp/tc-e2e';
const BASE = 'http://127.0.0.1:3100';
const browser = await chromium.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'] });
const results = [];
const secretsExtra = [];
const ok = (name) => { results.push(name); console.log('  ✔', name); };

async function newUser(handle) {
  const ctx = await browser.newContext({ baseURL: BASE });
  const page = await ctx.newPage();
  page.on('pageerror', (e) => console.log(`  [${handle}] pageerror:`, e.message));
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
for (const u of [bob, carol]) {
  await u.page.reload();
  await u.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().waitFor({ timeout: 15000 });
  await u.page.locator('.chat-name', { hasText: 'Top Secret Crew' }).first().click();
  await bubble(u.page, GSEC).waitFor({ timeout: 15000 });
}
ok('group: name and message decrypt for both members');
await send(carol, 'carol-in-group-7788');
await bubble(alice.page, 'carol-in-group-7788').waitFor({ timeout: 15000 });
ok('group: carol -> alice');

console.log('\n== Member leaves: admin rotates the group key, leaver is locked out ==');
const { DatabaseSync: DB2 } = await import('node:sqlite');
const peek = (sql) => { const d = new DB2(DIR + '/data/t.db', { readOnly: true }); const r = d.prepare(sql).all(); d.close(); return r; };
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

console.log('\n== Malicious server swaps a contact\'s key ==');
const { DatabaseSync } = await import('node:sqlite');
const dbh = new DatabaseSync(DIR + '/data/t.db');
const carolRow = dbh.prepare("select public_key_ed25519 e, public_key_x25519 x from users where handle='carol'").get();
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
