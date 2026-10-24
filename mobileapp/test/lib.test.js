import assert from 'node:assert/strict';
import test from 'node:test';
import {
  base64url, identify, isPrivateIPv4, keyMessage, loginError, loginState,
  parseAddress, pool, scanHosts,
} from '../www/lib.js';

test('parseAddress follows the desktop rules', () => {
  assert.equal(parseAddress(' 192.168.1.20:7717 '), 'http://192.168.1.20:7717');
  assert.equal(parseAddress('https://a.trycloudflare.com/'), 'https://a.trycloudflare.com');
  assert.equal(parseAddress('[fd00::1]:7717'), 'http://[fd00::1]:7717');
  for (const bad of ['', 'ftp://x', 'http://u:p@x', 'x:7717/path', 'x?q=1', 'x#f', 'x:99999']) {
    assert.throws(() => parseAddress(bad), Error, bad);
  }
});

test('identify accepts only the agenttik signature', () => {
  const ok = { application: 'agenttik', version: '1.2.3', name: 'box', id: 'abc' };
  assert.deepEqual(identify(200, ok), { version: '1.2.3', name: 'box', id: 'abc' });
  assert.deepEqual(identify(200, JSON.stringify(ok)), { version: '1.2.3', name: 'box', id: 'abc' });
  assert.equal(identify(404, ok), null);
  assert.equal(identify(200, { application: 'other', version: '1' }), null);
  assert.equal(identify(200, { application: 'agenttik', version: ' ' }), null);
  assert.equal(identify(200, '<html>'), null);
  assert.equal(identify(200, 'x'.repeat(5000)), null);
});

test('loginState reads the lock and 2FA from the front page', () => {
  assert.deepEqual(loginState('<div id="app"></div>'), { locked: false, totp: false });
  assert.deepEqual(loginState('<form method="post" action="/__auth/login"><input name="password">'), { locked: true, totp: false });
  assert.deepEqual(loginState('<form method="post" action="/__auth/login"><input id="code" name="code">'), { locked: true, totp: true });
});

test('loginError pulls the reason out of the form', () => {
  assert.equal(loginError('<p class="bad">Wrong password or code.</p>'), 'Wrong password or code.');
  assert.equal(loginError('<p class="bad">Settings &rsaquo; Server &amp; more</p>'), 'Settings › Server & more');
  assert.equal(loginError(''), 'Sign-in failed.');
});

test('scanHosts covers each private /24 once and skips the phone', () => {
  const hosts = scanHosts(['192.168.1.20', '192.168.1.30', '100.64.0.1', '10.0.0.5']);
  assert.equal(hosts.length, 252 + 253);
  assert.equal(hosts[0], '192.168.1.1');
  assert.ok(!hosts.includes('192.168.1.20') && !hosts.includes('192.168.1.30') && !hosts.includes('10.0.0.5'));
  assert.ok(hosts.includes('10.0.0.254'));
  assert.ok(!hosts.some((h) => h.startsWith('100.64.')));
  assert.deepEqual(scanHosts([]), []);
});

test('isPrivateIPv4', () => {
  for (const ip of ['10.1.2.3', '172.16.0.1', '172.31.255.1', '192.168.0.1']) assert.ok(isPrivateIPv4(ip), ip);
  for (const ip of ['172.32.0.1', '8.8.8.8', '127.0.0.1', '169.254.1.1', 'fe80::1', '1.2.3']) assert.ok(!isPrivateIPv4(ip), ip);
});

test('keyMessage matches netauth.KeyMessage', () => {
  assert.equal(new TextDecoder().decode(keyMessage('srv', 'n0')), 'agenttik-key-login-v1\nsrv\nn0');
});

test('base64url has no padding', () => {
  assert.equal(base64url(new Uint8Array([251, 255])), '-_8');
  assert.equal(base64url(new Uint8Array(32)).length, 43);
});

test('pool caps concurrency and visits every item', async () => {
  let live = 0;
  let peak = 0;
  const seen = [];
  await pool([1, 2, 3, 4, 5, 6, 7], 3, async (n) => {
    peak = Math.max(peak, ++live);
    await new Promise((r) => setTimeout(r, 5));
    seen.push(n);
    live--;
  });
  assert.equal(peak, 3);
  assert.deepEqual(seen.sort(), [1, 2, 3, 4, 5, 6, 7]);
});
