import test from 'node:test';
import assert from 'node:assert/strict';
import {
  newAccountKey, deriveIdentity, keysConsistent, formatAccountKey, parseAccountKey, safetyNumber, fingerprint,
} from '../src/lib/crypto/identity.js';
import {
  sealDirect, openDirect, sealGroup, openGroup, sealGroupKey, openGroupKey, sealGroupName, openGroupName, groupMessageEpoch,
} from '../src/lib/crypto/messages.js';
import { encryptFile, decryptFile, CHUNK } from '../src/lib/crypto/files.js';
import { randomBytes, utf8 } from '../src/lib/crypto/bytes.js';

function bigRandom(n) { const o = new Uint8Array(n); for (let i = 0; i < n; i += 65536) o.set(randomBytes(Math.min(65536, n - i)), i); return o; }
const mk = () => deriveIdentity(newAccountKey());
const ID = { alice: 'u-alice', bob: 'u-bob', carol: 'u-carol' };

test('identity is deterministic and its keys are consistent', () => {
  const seed = newAccountKey();
  const a = deriveIdentity(seed), b = deriveIdentity(seed);
  assert.deepEqual(a.edPub, b.edPub);
  assert.deepEqual(a.xPub, b.xPub);
  assert.ok(keysConsistent(a.edPub, a.xPub), 'published X25519 key must match the Ed25519 identity');
  assert.ok(!keysConsistent(a.edPub, mk().xPub));
  assert.notDeepEqual(a.authPub, a.edPub, 'login key must differ from identity key');
});

test('account key text round-trips and catches typos', () => {
  const seed = newAccountKey();
  const text = formatAccountKey(seed);
  assert.match(text, /^([A-Z2-9]{5}-){10}[A-Z2-9]{5}$/);
  assert.deepEqual(parseAccountKey(text), seed);
  assert.deepEqual(parseAccountKey(text.toLowerCase().replaceAll('-', ' ')), seed, 'tolerates case/spacing');
  const typo = text.slice(0, 3) + (text[3] === 'A' ? 'B' : 'A') + text.slice(4);
  assert.throws(() => parseAccountKey(typo), /typo/);
  assert.throws(() => parseAccountKey(text.slice(0, -6)), /length/);
});

test('safety numbers match from both sides and differ for other pairs', () => {
  const a = mk(), b = mk(), c = mk();
  assert.equal(safetyNumber(a.edPub, b.edPub), safetyNumber(b.edPub, a.edPub));
  assert.notEqual(safetyNumber(a.edPub, b.edPub), safetyNumber(a.edPub, c.edPub));
  assert.match(safetyNumber(a.edPub, b.edPub), /^(\d{5} ){11}\d{5}$/);
  assert.match(fingerprint(a.edPub), /^([0-9a-f]{4} ){3}[0-9a-f]{4}$/);
});

test('1:1: recipient reads it, sender reads it again (even on a brand-new device), server and others cannot', async () => {
  const alice = mk(), bob = mk(), carol = mk();
  const secret = 'meet at the old mill at 9 — password is hunter2';
  const { ciphertext, nonce } = await sealDirect({
    body: { type: 'text', text: secret }, identity: alice, senderId: ID.alice, recipientId: ID.bob, recipientXPub: bob.xPub,
  });
  assert.ok(!Buffer.from(ciphertext).includes(Buffer.from('hunter2')), 'no plaintext in ciphertext');

  const common = { ciphertext, nonce, senderId: ID.alice, recipientId: ID.bob, senderEdPub: alice.edPub };
  const atBob = await openDirect({ ...common, identity: bob, myId: ID.bob });
  assert.equal(atBob.text, secret);

  // Alice logs out, later signs in on a tablet using only her account key.
  const aliceTablet = deriveIdentity(alice.seed);
  const atAlice = await openDirect({ ...common, identity: aliceTablet, myId: ID.alice });
  assert.equal(atAlice.text, secret);

  await assert.rejects(openDirect({ ...common, identity: carol, myId: ID.bob }), 'a third party must not be able to read it');
  await assert.rejects(openDirect({ ...common, identity: carol, myId: ID.alice }));
});

test('1:1: tampering, re-routing and forgery are all detected', async () => {
  const alice = mk(), bob = mk(), mallory = mk();
  const { ciphertext, nonce } = await sealDirect({
    body: { type: 'text', text: 'pay bob $5' }, identity: alice, senderId: ID.alice, recipientId: ID.bob, recipientXPub: bob.xPub,
  });
  const base = { nonce, identity: bob, myId: ID.bob, senderId: ID.alice, recipientId: ID.bob, senderEdPub: alice.edPub };

  const flipped = new Uint8Array(ciphertext);
  flipped[flipped.length - 20] ^= 1;
  await assert.rejects(openDirect({ ...base, ciphertext: flipped }), 'bit flip');

  await assert.rejects(openDirect({ ...base, ciphertext, senderId: ID.carol }), 'server relabels the sender');
  await assert.rejects(openDirect({ ...base, ciphertext, recipientId: ID.carol }), 'server moves it to another conversation');
  await assert.rejects(openDirect({ ...base, ciphertext, senderEdPub: mallory.edPub }), 'wrong claimed signer');

  // Mallory (a legitimate user) builds a message to Bob but claims it is from Alice.
  const forged = await sealDirect({
    body: { type: 'text', text: 'I am Alice, send money' }, identity: mallory, senderId: ID.alice, recipientId: ID.bob, recipientXPub: bob.xPub,
  });
  await assert.rejects(openDirect({ ...base, ciphertext: forged.ciphertext, nonce: forged.nonce }), /Signature|forged/);
});

test('group: members read, outsiders and wrong epochs cannot; name is encrypted', async () => {
  const admin = mk(), bob = mk(), outsider = mk();
  const gid = 'group-1';
  const key0 = randomBytes(32);

  const envBob = await sealGroupKey({ groupKey: key0, groupId: gid, epoch: 0, memberId: ID.bob, memberXPub: bob.xPub, adminId: ID.alice, identity: admin });
  const got = await openGroupKey({ envelope: envBob, groupId: gid, epoch: 0, memberId: ID.bob, identity: bob, signerId: ID.alice, signerEdPub: admin.edPub });
  assert.deepEqual(got, key0);

  // a malicious server swaps in its own key, "signed" by someone who is not the admin
  const fakeKey = randomBytes(32);
  const fakeEnv = await sealGroupKey({ groupKey: fakeKey, groupId: gid, epoch: 0, memberId: ID.bob, memberXPub: bob.xPub, adminId: ID.alice, identity: outsider });
  await assert.rejects(openGroupKey({ envelope: fakeEnv, groupId: gid, epoch: 0, memberId: ID.bob, identity: bob, signerId: ID.alice, signerEdPub: admin.edPub }), /not signed/);
  // an envelope for Bob cannot be replayed for another epoch/member
  await assert.rejects(openGroupKey({ envelope: envBob, groupId: gid, epoch: 1, memberId: ID.bob, identity: bob, signerId: ID.alice, signerEdPub: admin.edPub }));

  const m = await sealGroup({ body: { type: 'text', text: 'launch codes: 0000' }, identity: admin, senderId: ID.alice, groupId: gid, epoch: 0, groupKey: key0 });
  assert.equal(groupMessageEpoch(m.ciphertext), 0);
  assert.ok(!Buffer.from(m.ciphertext).includes(Buffer.from('launch')));
  const open = (k, extra = {}) => openGroup({ ciphertext: m.ciphertext, nonce: m.nonce, senderId: ID.alice, groupId: gid, groupKey: k, senderEdPub: admin.edPub, ...extra });
  assert.equal((await open(key0)).text, 'launch codes: 0000');
  await assert.rejects(open(randomBytes(32)), 'wrong key');
  await assert.rejects(open(key0, { groupId: 'other-group' }), 'moved to another group');
  await assert.rejects(open(key0, { senderId: ID.bob }), 'relabelled sender');

  const nameBlob = await sealGroupName('Secret Project', gid, 0, key0);
  assert.ok(!Buffer.from(nameBlob).includes(Buffer.from('Secret')));
  assert.equal(await openGroupName(nameBlob, gid, key0), 'Secret Project');
  await assert.rejects(openGroupName(nameBlob, 'other-group', key0));
});

test('files: multi-chunk round trip, empty file, and all tampering detected', async () => {
  const data = bigRandom(CHUNK * 2 + 12345);
  const { blob, key } = await encryptFile(new Blob([data]));
  const raw = new Uint8Array(await blob.arrayBuffer());
  assert.equal(raw.length, data.length + 3 * 16);
  assert.ok(!Buffer.from(raw).includes(Buffer.from(data.subarray(100, 140))), 'no plaintext in blob');

  const back = new Uint8Array(await (await decryptFile(blob, key, data.length, 'image/png')).arrayBuffer());
  assert.deepEqual(back, data);

  const tampered = new Uint8Array(raw); tampered[500] ^= 1;
  await assert.rejects(decryptFile(new Blob([tampered]), key, data.length));
  await assert.rejects(decryptFile(new Blob([raw.subarray(0, CHUNK + 16)]), key, data.length), 'truncated');
  await assert.rejects(decryptFile(blob, randomBytes(32), data.length), 'wrong key');

  // drop the last frame and lie about the size so the length check passes: the "last frame" flag must catch it
  const frame = CHUNK + 16;
  const cut = new Blob([raw.subarray(0, frame * 2)]);
  await assert.rejects(decryptFile(cut, key, CHUNK * 2), 'truncated at a frame boundary');
  // swap frames 0 and 1
  const swapped = new Uint8Array(raw);
  swapped.set(raw.subarray(frame, frame * 2), 0); swapped.set(raw.subarray(0, frame), frame);
  await assert.rejects(decryptFile(new Blob([swapped]), key, data.length), 'reordered');

  const empty = await encryptFile(new Blob([]));
  assert.equal((await decryptFile(empty.blob, empty.key, 0)).size, 0);
});
