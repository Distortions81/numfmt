'use strict';

const Bits8 = 8;
const Bits16 = 16;
const Bits32 = 32;

function isValidTotalBits(totalBits) {
  return totalBits === Bits8 || totalBits === Bits16 || totalBits === Bits32;
}

function validateFinitePositive(name, value) {
  if (!(value > 0) || Number.isNaN(value) || !Number.isFinite(value)) {
    throw new Error(`${name} must be finite and > 0, got ${value}`);
  }
}

class Codec {
  constructor(totalBits, expBits, base, opts = {}) {
    if (!isValidTotalBits(totalBits)) {
      throw new Error(`totalBits must be one of [8,16,32], got ${totalBits}`);
    }
    if (expBits <= 0 || expBits >= totalBits) {
      throw new Error(`expBits must be in [1,${totalBits - 1}], got ${expBits}`);
    }
    validateFinitePositive('base', base);
    if (base <= 1) {
      throw new Error(`base must be finite and > 1, got ${base}`);
    }

    this.totalBits = totalBits;
    this.expBits = expBits;
    this.base = base;

    this.rangeMin = 0;
    this.rangeMax = 0;
    this.useRange = false;

    if (opts.useRange) {
      this.withRange(opts.rangeMin, opts.rangeMax);
    }
  }

  withRange(minValue, maxValue) {
    validateFinitePositive('minValue', minValue);
    if (!(maxValue > minValue) || Number.isNaN(maxValue) || !Number.isFinite(maxValue)) {
      throw new Error(`maxValue must be finite and > minValue, got ${maxValue}`);
    }
    this.rangeMin = minValue;
    this.rangeMax = maxValue;
    this.useRange = true;
    return this;
  }

  rangeEnabled() {
    return this.useRange;
  }

  mantBits() {
    return this.totalBits - this.expBits;
  }

  mantMask() {
    return Math.pow(2, this.mantBits()) - 1;
  }

  maxExp() {
    return Math.pow(2, this.expBits) - 1;
  }

  totalMask() {
    return Math.pow(2, this.totalBits) - 1;
  }

  maxCode() {
    return this.maxExp() * Math.pow(2, this.mantBits()) + this.mantMask();
  }

  encode(v) {
    if (!(v > 0) || Number.isNaN(v)) {
      return 0;
    }
    if (v === Infinity) {
      return this.maxCode();
    }
    return this.useRange ? this.encodeRanged(v) : this.encodeSI(v);
  }

  encodeSI(v) {
    const mantBits = this.mantBits();
    const mantMask = this.mantMask();
    const maxExp = this.maxExp();

    let exp = 0;
    let mant = v;
    while (mant >= this.base && exp < maxExp) {
      mant /= this.base;
      exp++;
    }

    if (exp === maxExp && mant >= this.base) {
      return this.maxCode();
    }
    if (mant < 1) {
      mant = 1;
    }
    if (mant >= this.base) {
      mant = this.base - Number.EPSILON;
    }

    const logT = Math.max(0, Math.min(1, Math.log(mant) / Math.log(this.base)));
    let q = 1;
    if (mantMask > 1) {
      q = 1 + Math.round(logT * (mantMask - 1));
    }
    if (q > mantMask) {
      q = mantMask;
    }
    return exp * Math.pow(2, mantBits) + q;
  }

  encodeRanged(v) {
    if (v <= this.rangeMin) {
      return 1;
    }
    if (v >= this.rangeMax) {
      return this.maxCode();
    }

    const levels = this.maxCode();
    if (levels <= 1) {
      return 1;
    }

    const logSpan = Math.log(this.rangeMax / this.rangeMin);
    const t = Math.max(0, Math.min(1, Math.log(v / this.rangeMin) / logSpan));

    let code = 1 + Math.round(t * (levels - 1));
    if (code < 1) {
      code = 1;
    }
    if (code > levels) {
      code = levels;
    }
    return code;
  }

  decode(code) {
    if (code === 0) {
      return 0;
    }
    const modulus = this.totalMask() + 1;
    const truncated = Math.trunc(code);
    const masked = ((truncated % modulus) + modulus) % modulus;
    return this.useRange ? this.decodeRanged(masked) : this.decodeSI(masked);
  }

  decodeSI(code) {
    const mantBits = this.mantBits();
    const mantMask = this.mantMask();
    const mantScale = Math.pow(2, mantBits);
    const exp = Math.floor(code / mantScale) % (this.maxExp() + 1);
    const q = code % mantScale;
    if (q === 0) {
      return 0;
    }

    const logT = mantMask > 1 ? (q - 1) / (mantMask - 1) : 0;
    const mant = Math.pow(this.base, logT);
    return mant * Math.pow(this.base, exp);
  }

  decodeRanged(code) {
    const levels = this.maxCode();
    if (levels <= 1) {
      return this.rangeMin;
    }
    if (code > levels) {
      code = levels;
    }
    if (code < 1) {
      return 0;
    }

    const t = (code - 1) / (levels - 1);
    return this.rangeMin * Math.exp(t * Math.log(this.rangeMax / this.rangeMin));
  }
}

function recommendedExpBits(totalBits, base, maxValue) {
  if (!isValidTotalBits(totalBits)) {
    throw new Error(`totalBits must be one of [8,16,32], got ${totalBits}`);
  }
  validateFinitePositive('base', base);
  if (base <= 1) {
    throw new Error(`base must be finite and > 1, got ${base}`);
  }
  validateFinitePositive('maxValue', maxValue);

  let requiredMaxExp = 0;
  if (maxValue >= 1) {
    requiredMaxExp = Math.floor(Math.log(maxValue) / Math.log(base) + 1e-12);
  }

  let b = 1;
  while (b < totalBits && (Math.pow(2, b) - 1) < requiredMaxExp) {
    b++;
  }
  if (b >= totalBits) {
    return totalBits - 1;
  }
  return b;
}

function newCodec(totalBits, expBits, base) {
  return new Codec(totalBits, expBits, base);
}

function newCodecWithRange(totalBits, expBits, base, minValue, maxValue) {
  return new Codec(totalBits, expBits, base).withRange(minValue, maxValue);
}

const defaultCodec = new Codec(Bits16, 3, 1000);

function encode(v) {
  return defaultCodec.encode(v);
}

function decode(code) {
  return defaultCodec.decode(code);
}

module.exports = {
  Bits8,
  Bits16,
  Bits32,
  Codec,
  defaultCodec,
  newCodec,
  newCodecWithRange,
  recommendedExpBits,
  encode,
  decode,
};
