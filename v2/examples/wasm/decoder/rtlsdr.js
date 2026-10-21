"use strict";

const RTL = (() => {
  const USBB = 1, SYSB = 2, IICB = 6;
  const USB_SYSCTL = 0x2000, USB_EPA_CTL = 0x2148, USB_EPA_MAXPKT = 0x2158;
  const DEMOD_CTL = 0x3000, DEMOD_CTL_1 = 0x300b;
  const XTAL = 28800000;
  const R820T_ADDR = 0x34;
  const IF_HZ = 3570000;
  const INIT = [0x83, 0x32, 0x75, 0xc0, 0x40, 0xd6, 0x6c, 0xf5, 0x63, 0x75, 0x68, 0x6c, 0x83, 0x80, 0x00, 0x0f, 0x00, 0xc0, 0x30, 0x48, 0xcc, 0x60, 0x00, 0x54, 0xae, 0x4a, 0xc0];
  const FIR = [-54, -36, -41, -40, -32, -14, 14, 53, 101, 156, 215, 273, 327, 372, 404, 421];
  const LNA_STEPS = [0, 9, 13, 40, 38, 13, 31, 22, 26, 31, 26, 14, 19, 5, 35, 13];
  const MIX_STEPS = [0, 5, 10, 10, 19, 9, 10, 25, 17, 10, 8, 16, 13, 6, 3, -8];
  const MUX = [
    [0, 0x08, 0x02, 0xdf], [50, 0x08, 0x02, 0xbe], [55, 0x08, 0x02, 0x8b], [60, 0x08, 0x02, 0x7b],
    [65, 0x08, 0x02, 0x69], [70, 0x08, 0x02, 0x58], [75, 0x00, 0x02, 0x44], [80, 0x00, 0x02, 0x44],
    [90, 0x00, 0x02, 0x34], [100, 0x00, 0x02, 0x34], [110, 0x00, 0x02, 0x24], [120, 0x00, 0x02, 0x24],
    [140, 0x00, 0x02, 0x14], [180, 0x00, 0x02, 0x13], [220, 0x00, 0x02, 0x13], [250, 0x00, 0x02, 0x11],
    [280, 0x00, 0x02, 0x00], [310, 0x00, 0x41, 0x00], [450, 0x00, 0x41, 0x00], [588, 0x00, 0x40, 0x00],
    [650, 0x00, 0x40, 0x00],
  ];
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const bitrev = (b) => {
    let r = 0;
    for (let i = 0; i < 8; i++) if (b & (1 << i)) r |= 0x80 >> i;
    return r;
  };

  class Device {
    constructor(usb) {
      this.usb = usb;
      this.regs = INIT.slice();
      this.running = false;
    }

    async out(value, index, bytes) {
      for (let i = 0; i < 3; i++) {
        const r = await this.usb.controlTransferOut({ requestType: "vendor", recipient: "device", request: 0, value, index }, new Uint8Array(bytes)).catch((e) => ({ status: e.message }));
        if (r.status === "ok") return true;
        this.stalls = (this.stalls || 0) + 1;
      }
      return false;
    }

    async in(value, index, len) {
      for (let i = 0; i < 3; i++) {
        const r = await this.usb.controlTransferIn({ requestType: "vendor", recipient: "device", request: 0, value, index }, len).catch((e) => ({ status: e.message }));
        if (r.status === "ok" && r.data.byteLength === len) return new Uint8Array(r.data.buffer);
        this.stalls = (this.stalls || 0) + 1;
      }
      return null;
    }

    static be(val, len) {
      return len === 1 ? [val & 0xff] : [(val >> 8) & 0xff, val & 0xff];
    }

    writeReg(block, addr, val, len) {
      return this.out(addr, (block << 8) | 0x10, Device.be(val, len));
    }

    async demodWrite(page, addr, val, len) {
      await this.out((addr << 8) | 0x20, 0x10 | page, Device.be(val, len));
      await this.in((0x01 << 8) | 0x20, 0x0a, 1);
    }

    i2cWrite(bytes) {
      return this.out(R820T_ADDR, (IICB << 8) | 0x10, bytes);
    }

    async i2cRead(reg, len) {
      await this.i2cWrite([reg]);
      return this.in(R820T_ADDR, IICB << 8, len);
    }

    repeater(on) {
      return this.demodWrite(1, 0x01, on ? 0x18 : 0x10, 1);
    }

    async tunerWrite(reg, vals) {
      for (let i = 0; i < vals.length; i += 7) {
        const part = vals.slice(i, i + 7);
        await this.i2cWrite([reg + i, ...part]);
        part.forEach((v, j) => {
          const k = reg + i + j - 5;
          if (k >= 0 && k < this.regs.length) this.regs[k] = v;
        });
      }
    }

    tunerMask(reg, val, mask) {
      const old = this.regs[reg - 5];
      return this.tunerWrite(reg, [(old & ~mask) | (val & mask)]);
    }

    async tunerRead(len) {
      const b = await this.i2cRead(0x00, len);
      return b ? Array.from(b, bitrev) : null;
    }

    async initBaseband() {
      await this.writeReg(USBB, USB_SYSCTL, 0x09, 1);
      await this.writeReg(USBB, USB_EPA_MAXPKT, 0x0002, 2);
      await this.writeReg(USBB, USB_EPA_CTL, 0x1002, 2);
      await this.writeReg(SYSB, DEMOD_CTL_1, 0x22, 1);
      await this.writeReg(SYSB, DEMOD_CTL, 0xe8, 1);
      await this.demodWrite(1, 0x01, 0x14, 1);
      await this.demodWrite(1, 0x01, 0x10, 1);
      await this.demodWrite(1, 0x15, 0x00, 1);
      await this.demodWrite(1, 0x16, 0x0000, 2);
      for (let i = 0; i < 6; i++) await this.demodWrite(1, 0x16 + i, 0x00, 1);
      const fir = new Array(20);
      for (let i = 0; i < 8; i++) fir[i] = FIR[i] & 0xff;
      for (let i = 0; i < 8; i += 2) {
        const a = FIR[8 + i], b = FIR[9 + i], k = 8 + (i * 3) / 2;
        fir[k] = (a >> 4) & 0xff;
        fir[k + 1] = ((a << 4) | ((b >> 8) & 0x0f)) & 0xff;
        fir[k + 2] = b & 0xff;
      }
      for (let i = 0; i < fir.length; i++) await this.demodWrite(1, 0x1c + i, fir[i], 1);
      await this.demodWrite(0, 0x19, 0x05, 1);
      await this.demodWrite(1, 0x93, 0xf0, 1);
      await this.demodWrite(1, 0x94, 0x0f, 1);
      await this.demodWrite(1, 0x11, 0x00, 1);
      await this.demodWrite(1, 0x04, 0x00, 1);
      await this.demodWrite(0, 0x61, 0x60, 1);
      await this.demodWrite(0, 0x06, 0x80, 1);
      await this.demodWrite(1, 0xb1, 0x1b, 1);
      await this.demodWrite(0, 0x0d, 0x83, 1);
    }

    async setIF(hz) {
      const f = -Math.floor((hz * 4194304) / XTAL);
      await this.demodWrite(1, 0x19, (f >> 16) & 0x3f, 1);
      await this.demodWrite(1, 0x1a, (f >> 8) & 0xff, 1);
      await this.demodWrite(1, 0x1b, f & 0xff, 1);
    }

    async setPLL(hz) {
      const khz = Math.floor((hz + 500) / 1000);
      await this.tunerMask(0x10, 0x00, 0x10);
      await this.tunerMask(0x1a, 0x00, 0x0c);
      await this.tunerMask(0x12, 0x80, 0xe0);
      let mixDiv = 2, divNum = 0;
      while (mixDiv <= 64) {
        if (khz * mixDiv >= 1770000 && khz * mixDiv < 3540000) {
          let d = mixDiv;
          while (d > 2) {
            d >>= 1;
            divNum++;
          }
          break;
        }
        mixDiv <<= 1;
      }
      const data = await this.tunerRead(5);
      const fine = data ? (data[4] & 0x30) >> 4 : 2;
      if (fine > 2) divNum--;
      else if (fine < 2) divNum++;
      await this.tunerMask(0x10, divNum << 5, 0xe0);
      const vco = hz * mixDiv;
      const nint = Math.floor(vco / (2 * XTAL));
      const sdm = Math.min(0xffff, Math.round((vco / (2 * XTAL) - nint) * 65536));
      if (nint > 63) throw new Error("R820T: no PLL setting for " + hz + " Hz");
      const ni = Math.floor((nint - 13) / 4);
      const si = nint - 4 * ni - 13;
      await this.tunerWrite(0x14, [(ni + (si << 6)) & 0xff]);
      await this.tunerMask(0x12, sdm === 0 ? 0x08 : 0x00, 0x08);
      await this.tunerWrite(0x16, [sdm >> 8]);
      await this.tunerWrite(0x15, [sdm & 0xff]);
      let locked = false;
      for (let i = 0; i < 2 && !locked; i++) {
        await sleep(2);
        const st = await this.tunerRead(3);
        locked = st !== null && (st[2] & 0x40) !== 0;
        if (!locked && i === 0) await this.tunerMask(0x12, 0x60, 0xe0);
      }
      this.locked = locked;
      if (!locked) console.warn("R820T: PLL not locked at " + hz + " Hz");
      await this.tunerMask(0x1a, 0x08, 0x08);
    }

    async tunerInit() {
      await this.tunerWrite(0x05, INIT);
      await this.tunerMask(0x10, 0x00, 0x08);
      await this.tunerMask(0x0c, 0x00, 0x0f);
      await this.tunerMask(0x13, 49, 0x3f);
      await this.tunerMask(0x1d, 0x00, 0x38);
      let cal = 0;
      for (let i = 0; i < 2; i++) {
        await this.tunerMask(0x0b, 0x6b, 0x60);
        await this.tunerMask(0x0f, 0x04, 0x04);
        await this.tunerMask(0x10, 0x00, 0x03);
        await this.setPLL(56000000);
        await this.tunerMask(0x0b, 0x10, 0x10);
        await sleep(1);
        await this.tunerMask(0x0b, 0x00, 0x10);
        await this.tunerMask(0x0f, 0x00, 0x04);
        const st = await this.tunerRead(5);
        cal = st ? st[4] & 0x0f : 0;
        if (cal && cal !== 0x0f) break;
      }
      if (cal === 0x0f) cal = 0;
      await this.tunerMask(0x0a, 0x10 | cal, 0x1f);
      await this.tunerMask(0x0b, 0x6b, 0xef);
      await this.tunerMask(0x07, 0x00, 0x80);
      await this.tunerMask(0x06, 0x10, 0x30);
      await this.tunerMask(0x1e, 0x60, 0x60);
      await this.tunerMask(0x05, 0x00, 0x80);
      await this.tunerMask(0x1f, 0x00, 0x80);
      await this.tunerMask(0x0f, 0x00, 0x80);
      await this.tunerMask(0x19, 0x60, 0x60);
      await this.tunerMask(0x1d, 0xe5, 0xc7);
      await this.tunerMask(0x1c, 0x24, 0xf8);
      await this.tunerWrite(0x0d, [0x53]);
      await this.tunerWrite(0x0e, [0x75]);
      await this.tunerMask(0x05, 0x00, 0x60);
      await this.tunerMask(0x06, 0x00, 0x08);
      await this.tunerMask(0x11, 0x38, 0x38);
      await this.tunerMask(0x17, 0x30, 0x30);
      await this.tunerMask(0x0a, 0x40, 0x60);
      await this.tunerMask(0x1d, 0x00, 0x38);
      await this.tunerMask(0x1c, 0x00, 0x04);
      await this.tunerMask(0x06, 0x00, 0x40);
      await this.tunerMask(0x1a, 0x30, 0x30);
      await this.repeater(false);
      await sleep(250);
      await this.repeater(true);
      await this.tunerMask(0x1d, 0x18, 0x38);
      await this.tunerMask(0x1c, 0x24, 0x04);
      await this.tunerMask(0x1e, 14, 0x1f);
      await this.tunerMask(0x1a, 0x20, 0x30);
    }

    async open() {
      if (this.usb.opened) await this.usb.close();
      await this.usb.open();
      if (this.usb.configuration === null) await this.usb.selectConfiguration(1);
      await this.usb.claimInterface(0);
      await this.initBaseband();
      await this.repeater(true);
      let id = -1;
      for (let i = 0; i < 4 && id < 0; i++) {
        const b = await this.i2cRead(0x00, 1);
        if (b) id = b[0];
        else await sleep(10);
      }
      if (id !== 0x69) {
        await this.repeater(false);
        throw new Error(`no R820T tuner at I2C address 0x34 (read ${id}): only the R820T and R820T2 are supported`);
      }
      await this.demodWrite(1, 0xb1, 0x1a, 1);
      await this.demodWrite(0, 0x08, 0x4d, 1);
      await this.setIF(IF_HZ);
      await this.demodWrite(1, 0x15, 0x01, 1);
      await this.tunerInit();
      await this.repeater(false);
    }

    async setRate(rate) {
      let ratio = Math.floor((XTAL * 4194304) / rate) & 0x0ffffffc;
      const real = (XTAL * 4194304) / (ratio | ((ratio & 0x08000000) << 1));
      await this.demodWrite(1, 0x9f, (ratio >>> 16) & 0xffff, 2);
      await this.demodWrite(1, 0xa1, ratio & 0xffff, 2);
      await this.demodWrite(1, 0x3f, 0, 1);
      await this.demodWrite(1, 0x3e, 0, 1);
      await this.demodWrite(1, 0x01, 0x14, 1);
      await this.demodWrite(1, 0x01, 0x10, 1);
      this.rate = real;
      return real;
    }

    async setFrequency(hz) {
      await this.repeater(true);
      try {
        const lo = hz + IF_HZ;
        const mhz = lo / 1e6;
        let i = 0;
        while (i < MUX.length - 1 && mhz >= MUX[i + 1][0]) i++;
        const [, openD, mux, tf] = MUX[i];
        await this.tunerMask(0x17, openD, 0x08);
        await this.tunerMask(0x1a, mux, 0xc3);
        await this.tunerWrite(0x1b, [tf]);
        await this.tunerMask(0x10, 0x00, 0x0b);
        await this.tunerMask(0x08, 0x00, 0x3f);
        await this.tunerMask(0x09, 0x00, 0x3f);
        await this.setPLL(lo);
      } finally {
        await this.repeater(false);
      }
    }

    async setGain(tenthsDB) {
      await this.repeater(true);
      try {
        if (tenthsDB < 0) {
          await this.tunerMask(0x05, 0x00, 0x10);
          await this.tunerMask(0x07, 0x10, 0x10);
          await this.tunerMask(0x0c, 0x0b, 0x9f);
          return -1;
        }
        await this.tunerMask(0x05, 0x10, 0x10);
        await this.tunerMask(0x07, 0x00, 0x10);
        await this.tunerMask(0x0c, 0x08, 0x9f);
        let total = 0, lna = 0, mix = 0;
        for (let i = 0; i < 15; i++) {
          if (total >= tenthsDB) break;
          total += LNA_STEPS[++lna];
          if (total >= tenthsDB) break;
          total += MIX_STEPS[++mix];
        }
        await this.tunerMask(0x05, lna, 0x0f);
        await this.tunerMask(0x07, mix, 0x0f);
        return total;
      } finally {
        await this.repeater(false);
      }
    }

    async start(onData, bufLen = 1 << 17, inflight = 6) {
      await this.writeReg(USBB, USB_EPA_CTL, 0x1002, 2);
      await this.writeReg(USBB, USB_EPA_CTL, 0x0000, 2);
      this.running = true;
      const loop = async () => {
        while (this.running) {
          const r = await this.usb.transferIn(1, bufLen);
          if (!this.running) break;
          if (r.status !== "ok") throw new Error("bulk transfer: " + r.status);
          onData(r.data.buffer.slice(r.data.byteOffset, r.data.byteOffset + r.data.byteLength));
        }
      };
      this.loops = Array.from({ length: inflight }, loop);
      return Promise.race(this.loops);
    }

    async stop() {
      this.running = false;
      try {
        await Promise.allSettled(this.loops || []);
        await this.usb.releaseInterface(0);
        await this.usb.close();
      } catch (e) {
        console.warn(e);
      }
    }
  }

  return {
    async request() {
      const usb = await navigator.usb.requestDevice({ filters: [{ vendorId: 0x0bda, productId: 0x2838 }, { vendorId: 0x0bda, productId: 0x2832 }] });
      return new Device(usb);
    },
    async granted() {
      const list = await navigator.usb.getDevices();
      const d = list.find((u) => u.vendorId === 0x0bda && (u.productId === 0x2838 || u.productId === 0x2832));
      return d ? new Device(d) : null;
    },
  };
})();
