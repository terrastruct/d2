"use strict";
(() => {
  const o = () => {
    const h = new Error("not implemented");
    return (h.code = "ENOSYS"), h;
  };
  if (!globalThis.fs) {
    let h = "";
    globalThis.fs = {
      constants: {
        O_WRONLY: -1,
        O_RDWR: -1,
        O_CREAT: -1,
        O_TRUNC: -1,
        O_APPEND: -1,
        O_EXCL: -1,
      },
      writeSync(n, s) {
        h += y.decode(s);
        const i = h.lastIndexOf(`
`);
        return (
          i != -1 && (console.log(h.substring(0, i)), (h = h.substring(i + 1))), s.length
        );
      },
      write(n, s, i, r, f, u) {
        if (i !== 0 || r !== s.length || f !== null) {
          u(o());
          return;
        }
        const d = this.writeSync(n, s);
        u(null, d);
      },
      chmod(n, s, i) {
        i(o());
      },
      chown(n, s, i, r) {
        r(o());
      },
      close(n, s) {
        s(o());
      },
      fchmod(n, s, i) {
        i(o());
      },
      fchown(n, s, i, r) {
        r(o());
      },
      fstat(n, s) {
        s(o());
      },
      fsync(n, s) {
        s(null);
      },
      ftruncate(n, s, i) {
        i(o());
      },
      lchown(n, s, i, r) {
        r(o());
      },
      link(n, s, i) {
        i(o());
      },
      lstat(n, s) {
        s(o());
      },
      mkdir(n, s, i) {
        i(o());
      },
      open(n, s, i, r) {
        r(o());
      },
      read(n, s, i, r, f, u) {
        u(o());
      },
      readdir(n, s) {
        s(o());
      },
      readlink(n, s) {
        s(o());
      },
      rename(n, s, i) {
        i(o());
      },
      rmdir(n, s) {
        s(o());
      },
      stat(n, s) {
        s(o());
      },
      symlink(n, s, i) {
        i(o());
      },
      truncate(n, s, i) {
        i(o());
      },
      unlink(n, s) {
        s(o());
      },
      utimes(n, s, i, r) {
        r(o());
      },
    };
  }
  if (
    (globalThis.process ||
      (globalThis.process = {
        getuid() {
          return -1;
        },
        getgid() {
          return -1;
        },
        geteuid() {
          return -1;
        },
        getegid() {
          return -1;
        },
        getgroups() {
          throw o();
        },
        pid: -1,
        ppid: -1,
        umask() {
          throw o();
        },
        cwd() {
          throw o();
        },
        chdir() {
          throw o();
        },
      }),
    !globalThis.crypto)
  )
    throw new Error(
      "globalThis.crypto is not available, polyfill required (crypto.getRandomValues only)"
    );
  if (!globalThis.performance)
    throw new Error(
      "globalThis.performance is not available, polyfill required (performance.now only)"
    );
  if (!globalThis.TextEncoder)
    throw new Error("globalThis.TextEncoder is not available, polyfill required");
  if (!globalThis.TextDecoder)
    throw new Error("globalThis.TextDecoder is not available, polyfill required");
  const g = new TextEncoder("utf-8"),
    y = new TextDecoder("utf-8");
  globalThis.Go = class {
    constructor() {
      (this.argv = ["js"]),
        (this.env = {}),
        (this.exit = (t) => {
          t !== 0 && console.warn("exit code:", t);
        }),
        (this._exitPromise = new Promise((t) => {
          this._resolveExitPromise = t;
        })),
        (this._pendingEvent = null),
        (this._scheduledTimeouts = new Map()),
        (this._nextCallbackTimeoutID = 1);
      const h = (t, e) => {
          this.mem.setUint32(t + 0, e, !0),
            this.mem.setUint32(t + 4, Math.floor(e / 4294967296), !0);
        },
        n = (t, e) => {
          this.mem.setUint32(t + 0, e, !0);
        },
        s = (t) => {
          const e = this.mem.getUint32(t + 0, !0),
            l = this.mem.getInt32(t + 4, !0);
          return e + l * 4294967296;
        },
        i = (t) => {
          const e = this.mem.getFloat64(t, !0);
          if (e === 0) return;
          if (!isNaN(e)) return e;
          const l = this.mem.getUint32(t, !0);
          return this._values[l];
        },
        r = (t, e) => {
          if (typeof e == "number" && e !== 0) {
            if (isNaN(e)) {
              this.mem.setUint32(t + 4, 2146959360, !0), this.mem.setUint32(t, 0, !0);
              return;
            }
            this.mem.setFloat64(t, e, !0);
            return;
          }
          if (e === void 0) {
            this.mem.setFloat64(t, 0, !0);
            return;
          }
          let a = this._ids.get(e);
          a === void 0 &&
            ((a = this._idPool.pop()),
            a === void 0 && (a = this._values.length),
            (this._values[a] = e),
            (this._goRefCounts[a] = 0),
            this._ids.set(e, a)),
            this._goRefCounts[a]++;
          let c = 0;
          switch (typeof e) {
            case "object":
              e !== null && (c = 1);
              break;
            case "string":
              c = 2;
              break;
            case "symbol":
              c = 3;
              break;
            case "function":
              c = 4;
              break;
          }
          this.mem.setUint32(t + 4, 2146959360 | c, !0), this.mem.setUint32(t, a, !0);
        },
        f = (t) => {
          const e = s(t + 0),
            l = s(t + 8);
          return new Uint8Array(this._inst.exports.mem.buffer, e, l);
        },
        u = (t) => {
          const e = s(t + 0),
            l = s(t + 8),
            a = new Array(l);
          for (let c = 0; c < l; c++) a[c] = i(e + c * 8);
          return a;
        },
        d = (t) => {
          const e = s(t + 0),
            l = s(t + 8);
          return y.decode(new DataView(this._inst.exports.mem.buffer, e, l));
        },
        m = Date.now() - performance.now();
      this.importObject = {
        _gotest: { add: (t, e) => t + e },
        gojs: {
          "runtime.wasmExit": (t) => {
            t >>>= 0;
            const e = this.mem.getInt32(t + 8, !0);
            (this.exited = !0),
              delete this._inst,
              delete this._values,
              delete this._goRefCounts,
              delete this._ids,
              delete this._idPool,
              this.exit(e);
          },
          "runtime.wasmWrite": (t) => {
            t >>>= 0;
            const e = s(t + 8),
              l = s(t + 16),
              a = this.mem.getInt32(t + 24, !0);
            fs.writeSync(e, new Uint8Array(this._inst.exports.mem.buffer, l, a));
          },
          "runtime.resetMemoryDataView": (t) => {
            (t >>>= 0), (this.mem = new DataView(this._inst.exports.mem.buffer));
          },
          "runtime.nanotime1": (t) => {
            (t >>>= 0), h(t + 8, (m + performance.now()) * 1e6);
          },
          "runtime.walltime": (t) => {
            t >>>= 0;
            const e = new Date().getTime();
            h(t + 8, e / 1e3), this.mem.setInt32(t + 16, (e % 1e3) * 1e6, !0);
          },
          "runtime.scheduleTimeoutEvent": (t) => {
            t >>>= 0;
            const e = this._nextCallbackTimeoutID;
            this._nextCallbackTimeoutID++,
              this._scheduledTimeouts.set(
                e,
                setTimeout(() => {
                  for (this._resume(); this._scheduledTimeouts.has(e); )
                    console.warn("scheduleTimeoutEvent: missed timeout event"),
                      this._resume();
                }, s(t + 8))
              ),
              this.mem.setInt32(t + 16, e, !0);
          },
          "runtime.clearTimeoutEvent": (t) => {
            t >>>= 0;
            const e = this.mem.getInt32(t + 8, !0);
            clearTimeout(this._scheduledTimeouts.get(e)),
              this._scheduledTimeouts.delete(e);
          },
          "runtime.getRandomData": (t) => {
            (t >>>= 0), crypto.getRandomValues(f(t + 8));
          },
          "syscall/js.finalizeRef": (t) => {
            t >>>= 0;
            const e = this.mem.getUint32(t + 8, !0);
            if ((this._goRefCounts[e]--, this._goRefCounts[e] === 0)) {
              const l = this._values[e];
              (this._values[e] = null), this._ids.delete(l), this._idPool.push(e);
            }
          },
          "syscall/js.stringVal": (t) => {
            (t >>>= 0), r(t + 24, d(t + 8));
          },
          "syscall/js.valueGet": (t) => {
            t >>>= 0;
            const e = Reflect.get(i(t + 8), d(t + 16));
            (t = this._inst.exports.getsp() >>> 0), r(t + 32, e);
          },
          "syscall/js.valueSet": (t) => {
            (t >>>= 0), Reflect.set(i(t + 8), d(t + 16), i(t + 32));
          },
          "syscall/js.valueDelete": (t) => {
            (t >>>= 0), Reflect.deleteProperty(i(t + 8), d(t + 16));
          },
          "syscall/js.valueIndex": (t) => {
            (t >>>= 0), r(t + 24, Reflect.get(i(t + 8), s(t + 16)));
          },
          "syscall/js.valueSetIndex": (t) => {
            (t >>>= 0), Reflect.set(i(t + 8), s(t + 16), i(t + 24));
          },
          "syscall/js.valueCall": (t) => {
            t >>>= 0;
            try {
              const e = i(t + 8),
                l = Reflect.get(e, d(t + 16)),
                a = u(t + 32),
                c = Reflect.apply(l, e, a);
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 56, c),
                this.mem.setUint8(t + 64, 1);
            } catch (e) {
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 56, e),
                this.mem.setUint8(t + 64, 0);
            }
          },
          "syscall/js.valueInvoke": (t) => {
            t >>>= 0;
            try {
              const e = i(t + 8),
                l = u(t + 16),
                a = Reflect.apply(e, void 0, l);
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 40, a),
                this.mem.setUint8(t + 48, 1);
            } catch (e) {
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 40, e),
                this.mem.setUint8(t + 48, 0);
            }
          },
          "syscall/js.valueNew": (t) => {
            t >>>= 0;
            try {
              const e = i(t + 8),
                l = u(t + 16),
                a = Reflect.construct(e, l);
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 40, a),
                this.mem.setUint8(t + 48, 1);
            } catch (e) {
              (t = this._inst.exports.getsp() >>> 0),
                r(t + 40, e),
                this.mem.setUint8(t + 48, 0);
            }
          },
          "syscall/js.valueLength": (t) => {
            (t >>>= 0), h(t + 16, parseInt(i(t + 8).length));
          },
          "syscall/js.valuePrepareString": (t) => {
            t >>>= 0;
            const e = g.encode(String(i(t + 8)));
            r(t + 16, e), h(t + 24, e.length);
          },
          "syscall/js.valueLoadString": (t) => {
            t >>>= 0;
            const e = i(t + 8);
            f(t + 16).set(e);
          },
          "syscall/js.valueInstanceOf": (t) => {
            (t >>>= 0), this.mem.setUint8(t + 24, i(t + 8) instanceof i(t + 16) ? 1 : 0);
          },
          "syscall/js.copyBytesToGo": (t) => {
            t >>>= 0;
            const e = f(t + 8),
              l = i(t + 32);
            if (!(l instanceof Uint8Array || l instanceof Uint8ClampedArray)) {
              this.mem.setUint8(t + 48, 0);
              return;
            }
            const a = l.subarray(0, e.length);
            e.set(a), h(t + 40, a.length), this.mem.setUint8(t + 48, 1);
          },
          "syscall/js.copyBytesToJS": (t) => {
            t >>>= 0;
            const e = i(t + 8),
              l = f(t + 16);
            if (!(e instanceof Uint8Array || e instanceof Uint8ClampedArray)) {
              this.mem.setUint8(t + 48, 0);
              return;
            }
            const a = l.subarray(0, e.length);
            e.set(a), h(t + 40, a.length), this.mem.setUint8(t + 48, 1);
          },
          debug: (t) => {
            console.log(t);
          },
        },
      };
    }
    async run(h) {
      if (!(h instanceof WebAssembly.Instance))
        throw new Error("Go.run: WebAssembly.Instance expected");
      (this._inst = h),
        (this.mem = new DataView(this._inst.exports.mem.buffer)),
        (this._values = [NaN, 0, null, !0, !1, globalThis, this]),
        (this._goRefCounts = new Array(this._values.length).fill(1 / 0)),
        (this._ids = new Map([
          [0, 1],
          [null, 2],
          [!0, 3],
          [!1, 4],
          [globalThis, 5],
          [this, 6],
        ])),
        (this._idPool = []),
        (this.exited = !1);
      let n = 4096;
      const s = (m) => {
          const t = n,
            e = g.encode(m + "\0");
          return (
            new Uint8Array(this.mem.buffer, n, e.length).set(e),
            (n += e.length),
            n % 8 !== 0 && (n += 8 - (n % 8)),
            t
          );
        },
        i = this.argv.length,
        r = [];
      this.argv.forEach((m) => {
        r.push(s(m));
      }),
        r.push(0),
        Object.keys(this.env)
          .sort()
          .forEach((m) => {
            r.push(s(`${m}=${this.env[m]}`));
          }),
        r.push(0);
      const u = n;
      if (
        (r.forEach((m) => {
          this.mem.setUint32(n, m, !0), this.mem.setUint32(n + 4, 0, !0), (n += 8);
        }),
        n >= 12288)
      )
        throw new Error(
          "total length of command line and environment variables exceeds limit"
        );
      this._inst.exports.run(i, u),
        this.exited && this._resolveExitPromise(),
        await this._exitPromise;
    }
    _resume() {
      if (this.exited) throw new Error("Go program has already exited");
      this._inst.exports.resume(), this.exited && this._resolveExitPromise();
    }
    _makeFuncWrapper(h) {
      const n = this;
      return function () {
        const s = { id: h, this: this, args: arguments };
        return (n._pendingEvent = s), n._resume(), s.result;
      };
    }
  };
})();
