'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

const {
  Bits8,
  Bits16,
  Bits32,
  Codec,
  encode,
  decode,
  newCodecWithRange,
  recommendedExpBits,
} = require('./index');

test('default encode/decode for zero', () => {
  assert.equal(encode(0), 0);
  assert.equal(decode(0), 0);
});

test('default encode/decode relative error is small', () => {
  const values = [1, 12.345, 999.9, 1000, 1234.56, 999999, 1234567, 75000000000];
  for (const v of values) {
    const code = encode(v);
    const got = decode(code);
    const relErr = Math.abs(got - v) / v;
    assert.ok(relErr <= 1e-3, `v=${v} code=${code} got=${got} relErr=${relErr}`);
  }
});

test('codec monotonic', () => {
  const c = new Codec(Bits16, 3, 1000);
  const values = [0, 1, 2, 10, 999.9, 1000, 1001, 10000, 1000000, 1000000000];

  let prev = 0;
  for (const v of values) {
    const code = c.encode(v);
    assert.ok(code >= prev, `v=${v} code=${code} prev=${prev}`);
    prev = code;
  }
});

test('bits variants work', () => {
  const codecs = [
    { c: new Codec(Bits8, 3, 1000), maxRelErr: 0.2 },
    { c: new Codec(Bits16, 3, 1000), maxRelErr: 1e-3 },
    { c: new Codec(Bits32, 6, 1000), maxRelErr: 2e-6 },
  ];
  const values = [1, 3.14159, 42, 999.9, 1000, 10001, 1234567, 75000000000];

  for (const { c, maxRelErr } of codecs) {
    for (const v of values) {
      const code = c.encode(v);
      const got = c.decode(code);
      const relErr = Math.abs(got - v) / v;
      assert.ok(relErr <= maxRelErr, `bits=${c.totalBits} v=${v} relErr=${relErr}`);
    }
  }
});

test('range codec improves bounded-domain average error', () => {
  const unbounded = new Codec(Bits16, 3, 1000);
  const ranged = newCodecWithRange(Bits16, 3, 1000, 10000, 200000);

  let unboundedErr = 0;
  let rangedErr = 0;
  const samples = 300;

  for (let i = 0; i < samples; i++) {
    const t = i / (samples - 1);
    const v = 10000 * Math.exp(t * Math.log(200000 / 10000));
    const u = unbounded.decode(unbounded.encode(v));
    const r = ranged.decode(ranged.encode(v));
    unboundedErr += Math.abs(u - v) / v;
    rangedErr += Math.abs(r - v) / v;
  }

  assert.ok(rangedErr < unboundedErr, `unbounded=${unboundedErr} ranged=${rangedErr}`);
});

test('recommended exp bits', () => {
  assert.equal(recommendedExpBits(Bits8, 1000, 1000000), 2);
  assert.equal(recommendedExpBits(Bits16, 1000, 1000000000), 2);
  assert.equal(recommendedExpBits(Bits32, 1000, 1000000000000), 3);
});
