/*eslint-disable */
// rough.js is from https://github.com/rough-stuff/rough.
// Attribution for this file is as follows:
//
// MIT License
//
// Copyright (c) 2019 Preet Shihn
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
const t = "http://www.w3.org/2000/svg";
function e(t, e, s) {
  if (t && t.length) {
    const [n, i] = e,
      a = (Math.PI / 180) * s,
      o = Math.cos(a),
      h = Math.sin(a);
    t.forEach((t) => {
      const [e, s] = t;
      (t[0] = (e - n) * o - (s - i) * h + n), (t[1] = (e - n) * h + (s - i) * o + i);
    });
  }
}
function s(t) {
  const e = t[0],
    s = t[1];
  return Math.sqrt(Math.pow(e[0] - s[0], 2) + Math.pow(e[1] - s[1], 2));
}
function n(t, e) {
  return t.type === e;
}
const i = {
  A: 7,
  a: 7,
  C: 6,
  c: 6,
  H: 1,
  h: 1,
  L: 2,
  l: 2,
  M: 2,
  m: 2,
  Q: 4,
  q: 4,
  S: 4,
  s: 4,
  T: 4,
  t: 2,
  V: 1,
  v: 1,
  Z: 0,
  z: 0,
};
class a {
  constructor(t) {
    (this.COMMAND = 0),
      (this.NUMBER = 1),
      (this.EOD = 2),
      (this.segments = []),
      this.parseData(t),
      this.processPoints();
  }
  tokenize(t) {
    const e = new Array();
    for (; "" !== t; )
      if (t.match(/^([ \t\r\n,]+)/)) t = t.substr(RegExp.$1.length);
      else if (t.match(/^([aAcChHlLmMqQsStTvVzZ])/))
        (e[e.length] = { type: this.COMMAND, text: RegExp.$1 }),
          (t = t.substr(RegExp.$1.length));
      else {
        if (!t.match(/^(([-+]?[0-9]+(\.[0-9]*)?|[-+]?\.[0-9]+)([eE][-+]?[0-9]+)?)/))
          return [];
        (e[e.length] = { type: this.NUMBER, text: `${parseFloat(RegExp.$1)}` }),
          (t = t.substr(RegExp.$1.length));
      }
    return (e[e.length] = { type: this.EOD, text: "" }), e;
  }
  parseData(t) {
    const e = this.tokenize(t);
    let s = 0,
      a = e[s],
      o = "BOD";
    for (this.segments = new Array(); !n(a, this.EOD); ) {
      let h;
      const r = new Array();
      if ("BOD" === o) {
        if ("M" !== a.text && "m" !== a.text) return void this.parseData("M0,0" + t);
        s++, (h = i[a.text]), (o = a.text);
      } else n(a, this.NUMBER) ? (h = i[o]) : (s++, (h = i[a.text]), (o = a.text));
      if (s + h < e.length) {
        for (let t = s; t < s + h; t++) {
          const s = e[t];
          if (!n(s, this.NUMBER))
            return void console.error("Param not a number: " + o + "," + s.text);
          r[r.length] = +s.text;
        }
        if ("number" != typeof i[o]) return void console.error("Bad segment: " + o);
        {
          const t = { key: o, data: r };
          this.segments.push(t),
            (s += h),
            (a = e[s]),
            "M" === o && (o = "L"),
            "m" === o && (o = "l");
        }
      } else console.error("Path data ended short");
    }
  }
  get closed() {
    if (void 0 === this._closed) {
      this._closed = !1;
      for (const t of this.segments) "z" === t.key.toLowerCase() && (this._closed = !0);
    }
    return this._closed;
  }
  processPoints() {
    let t = null,
      e = [0, 0];
    for (let s = 0; s < this.segments.length; s++) {
      const n = this.segments[s];
      switch (n.key) {
        case "M":
        case "L":
        case "T":
          n.point = [n.data[0], n.data[1]];
          break;
        case "m":
        case "l":
        case "t":
          n.point = [n.data[0] + e[0], n.data[1] + e[1]];
          break;
        case "H":
          n.point = [n.data[0], e[1]];
          break;
        case "h":
          n.point = [n.data[0] + e[0], e[1]];
          break;
        case "V":
          n.point = [e[0], n.data[0]];
          break;
        case "v":
          n.point = [e[0], n.data[0] + e[1]];
          break;
        case "z":
        case "Z":
          t && (n.point = [t[0], t[1]]);
          break;
        case "C":
          n.point = [n.data[4], n.data[5]];
          break;
        case "c":
          n.point = [n.data[4] + e[0], n.data[5] + e[1]];
          break;
        case "S":
          n.point = [n.data[2], n.data[3]];
          break;
        case "s":
          n.point = [n.data[2] + e[0], n.data[3] + e[1]];
          break;
        case "Q":
          n.point = [n.data[2], n.data[3]];
          break;
        case "q":
          n.point = [n.data[2] + e[0], n.data[3] + e[1]];
          break;
        case "A":
          n.point = [n.data[5], n.data[6]];
          break;
        case "a":
          n.point = [n.data[5] + e[0], n.data[6] + e[1]];
      }
      ("m" !== n.key && "M" !== n.key) || (t = null),
        n.point && ((e = n.point), t || (t = n.point)),
        ("z" !== n.key && "Z" !== n.key) || (t = null);
    }
  }
}
class o {
  constructor(t) {
    (this._position = [0, 0]),
      (this._first = null),
      (this.bezierReflectionPoint = null),
      (this.quadReflectionPoint = null),
      (this.parsed = new a(t));
  }
  get segments() {
    return this.parsed.segments;
  }
  get closed() {
    return this.parsed.closed;
  }
  get linearPoints() {
    if (!this._linearPoints) {
      const t = [];
      let e = [];
      for (const s of this.parsed.segments) {
        const n = s.key.toLowerCase();
        (("m" !== n && "z" !== n) || (e.length && (t.push(e), (e = [])), "z" !== n)) &&
          s.point &&
          e.push(s.point);
      }
      e.length && (t.push(e), (e = [])), (this._linearPoints = t);
    }
    return this._linearPoints;
  }
  get first() {
    return this._first;
  }
  set first(t) {
    this._first = t;
  }
  setPosition(t, e) {
    (this._position = [t, e]), this._first || (this._first = [t, e]);
  }
  get position() {
    return this._position;
  }
  get x() {
    return this._position[0];
  }
  get y() {
    return this._position[1];
  }
}
class h {
  constructor(t, e, s, n, i, a) {
    if (
      ((this._segIndex = 0),
      (this._numSegs = 0),
      (this._rx = 0),
      (this._ry = 0),
      (this._sinPhi = 0),
      (this._cosPhi = 0),
      (this._C = [0, 0]),
      (this._theta = 0),
      (this._delta = 0),
      (this._T = 0),
      (this._from = t),
      t[0] === e[0] && t[1] === e[1])
    )
      return;
    const o = Math.PI / 180;
    (this._rx = Math.abs(s[0])),
      (this._ry = Math.abs(s[1])),
      (this._sinPhi = Math.sin(n * o)),
      (this._cosPhi = Math.cos(n * o));
    const h = (this._cosPhi * (t[0] - e[0])) / 2 + (this._sinPhi * (t[1] - e[1])) / 2,
      r = (-this._sinPhi * (t[0] - e[0])) / 2 + (this._cosPhi * (t[1] - e[1])) / 2;
    let c = 0;
    const l =
      this._rx * this._rx * this._ry * this._ry -
      this._rx * this._rx * r * r -
      this._ry * this._ry * h * h;
    if (l < 0) {
      const t = Math.sqrt(1 - l / (this._rx * this._rx * this._ry * this._ry));
      (this._rx = this._rx * t), (this._ry = this._ry * t), (c = 0);
    } else
      c =
        (i === a ? -1 : 1) *
        Math.sqrt(l / (this._rx * this._rx * r * r + this._ry * this._ry * h * h));
    const u = (c * this._rx * r) / this._ry,
      p = (-c * this._ry * h) / this._rx;
    (this._C = [0, 0]),
      (this._C[0] = this._cosPhi * u - this._sinPhi * p + (t[0] + e[0]) / 2),
      (this._C[1] = this._sinPhi * u + this._cosPhi * p + (t[1] + e[1]) / 2),
      (this._theta = this.calculateVectorAngle(
        1,
        0,
        (h - u) / this._rx,
        (r - p) / this._ry
      ));
    let d = this.calculateVectorAngle(
      (h - u) / this._rx,
      (r - p) / this._ry,
      (-h - u) / this._rx,
      (-r - p) / this._ry
    );
    !a && d > 0 ? (d -= 2 * Math.PI) : a && d < 0 && (d += 2 * Math.PI),
      (this._numSegs = Math.ceil(Math.abs(d / (Math.PI / 2)))),
      (this._delta = d / this._numSegs),
      (this._T =
        ((8 / 3) * Math.sin(this._delta / 4) * Math.sin(this._delta / 4)) /
        Math.sin(this._delta / 2));
  }
  getNextSegment() {
    if (this._segIndex === this._numSegs) return null;
    const t = Math.cos(this._theta),
      e = Math.sin(this._theta),
      s = this._theta + this._delta,
      n = Math.cos(s),
      i = Math.sin(s),
      a = [
        this._cosPhi * this._rx * n - this._sinPhi * this._ry * i + this._C[0],
        this._sinPhi * this._rx * n + this._cosPhi * this._ry * i + this._C[1],
      ],
      o = [
        this._from[0] +
          this._T * (-this._cosPhi * this._rx * e - this._sinPhi * this._ry * t),
        this._from[1] +
          this._T * (-this._sinPhi * this._rx * e + this._cosPhi * this._ry * t),
      ],
      h = [
        a[0] + this._T * (this._cosPhi * this._rx * i + this._sinPhi * this._ry * n),
        a[1] + this._T * (this._sinPhi * this._rx * i - this._cosPhi * this._ry * n),
      ];
    return (
      (this._theta = s),
      (this._from = [a[0], a[1]]),
      this._segIndex++,
      { cp1: o, cp2: h, to: a }
    );
  }
  calculateVectorAngle(t, e, s, n) {
    const i = Math.atan2(e, t),
      a = Math.atan2(n, s);
    return a >= i ? a - i : 2 * Math.PI - (i - a);
  }
}
class r {
  constructor(t, e) {
    (this.sets = t), (this.closed = e);
  }
  fit(t) {
    const e = [];
    for (const s of this.sets) {
      const n = s.length;
      let i = Math.floor(t * n);
      if (i < 5) {
        if (n <= 5) continue;
        i = 5;
      }
      e.push(this.reduce(s, i));
    }
    let s = "";
    for (const t of e) {
      for (let e = 0; e < t.length; e++) {
        const n = t[e];
        s += 0 === e ? "M" + n[0] + "," + n[1] : "L" + n[0] + "," + n[1];
      }
      this.closed && (s += "z ");
    }
    return s;
  }
  reduce(t, e) {
    if (t.length <= e) return t;
    const n = t.slice(0);
    for (; n.length > e; ) {
      let t = -1,
        e = -1;
      for (let i = 1; i < n.length - 1; i++) {
        const a = s([n[i - 1], n[i]]),
          o = s([n[i], n[i + 1]]),
          h = s([n[i - 1], n[i + 1]]),
          r = (a + o + h) / 2,
          c = Math.sqrt(r * (r - a) * (r - o) * (r - h));
        (t < 0 || c < t) && ((t = c), (e = i));
      }
      if (!(e > 0)) break;
      n.splice(e, 1);
    }
    return n;
  }
}
function c(t, s) {
  const n = [0, 0],
    i = Math.round(s.hachureAngle + 90);
  i && e(t, n, i);
  const a = (function (t, e) {
    const s = [...t];
    s[0].join(",") !== s[s.length - 1].join(",") && s.push([s[0][0], s[0][1]]);
    const n = [];
    if (s && s.length > 2) {
      let t = e.hachureGap;
      t < 0 && (t = 4 * e.strokeWidth), (t = Math.max(t, 0.1));
      const i = [];
      for (let t = 0; t < s.length - 1; t++) {
        const e = s[t],
          n = s[t + 1];
        if (e[1] !== n[1]) {
          const t = Math.min(e[1], n[1]);
          i.push({
            ymin: t,
            ymax: Math.max(e[1], n[1]),
            x: t === e[1] ? e[0] : n[0],
            islope: (n[0] - e[0]) / (n[1] - e[1]),
          });
        }
      }
      if (
        (i.sort((t, e) =>
          t.ymin < e.ymin
            ? -1
            : t.ymin > e.ymin
            ? 1
            : t.x < e.x
            ? -1
            : t.x > e.x
            ? 1
            : t.ymax === e.ymax
            ? 0
            : (t.ymax - e.ymax) / Math.abs(t.ymax - e.ymax)
        ),
        !i.length)
      )
        return n;
      let a = [],
        o = i[0].ymin;
      for (; a.length || i.length; ) {
        if (i.length) {
          let t = -1;
          for (let e = 0; e < i.length && !(i[e].ymin > o); e++) t = e;
          i.splice(0, t + 1).forEach((t) => {
            a.push({ s: o, edge: t });
          });
        }
        if (
          ((a = a.filter((t) => !(t.edge.ymax <= o))),
          a.sort((t, e) =>
            t.edge.x === e.edge.x
              ? 0
              : (t.edge.x - e.edge.x) / Math.abs(t.edge.x - e.edge.x)
          ),
          a.length > 1)
        )
          for (let t = 0; t < a.length; t += 2) {
            const e = t + 1;
            if (e >= a.length) break;
            const s = a[t].edge,
              i = a[e].edge;
            n.push([
              [Math.round(s.x), o],
              [Math.round(i.x), o],
            ]);
          }
        (o += t),
          a.forEach((e) => {
            e.edge.x = e.edge.x + t * e.edge.islope;
          });
      }
    }
    return n;
  })(t, s);
  return (
    i &&
      (e(t, n, -i),
      (function (t, s, n) {
        const i = [];
        t.forEach((t) => i.push(...t)), e(i, s, n);
      })(a, n, -i)),
    a
  );
}
class l {
  constructor(t) {
    this.helper = t;
  }
  fillPolygon(t, e) {
    return this._fillPolygon(t, e);
  }
  _fillPolygon(t, e, s = !1) {
    const n = c(t, e);
    return { type: "fillSketch", ops: this.renderLines(n, e, s) };
  }
  renderLines(t, e, s) {
    let n = [],
      i = null;
    for (const a of t)
      (n = n.concat(this.helper.doubleLineOps(a[0][0], a[0][1], a[1][0], a[1][1], e))),
        s &&
          i &&
          (n = n.concat(this.helper.doubleLineOps(i[0], i[1], a[0][0], a[0][1], e))),
        (i = a[1]);
    return n;
  }
}
class u extends l {
  fillPolygon(t, e) {
    return this._fillPolygon(t, e, !0);
  }
}
class p extends l {
  fillPolygon(t, e) {
    const s = this._fillPolygon(t, e),
      n = Object.assign({}, e, { hachureAngle: e.hachureAngle + 90 }),
      i = this._fillPolygon(t, n);
    return (s.ops = s.ops.concat(i.ops)), s;
  }
}
class d {
  constructor(t) {
    this.helper = t;
  }
  fillPolygon(t, e) {
    const s = c(
      t,
      (e = Object.assign({}, e, {
        curveStepCount: 4,
        hachureAngle: 0,
        roughness: 1,
      }))
    );
    return this.dotsOnLines(s, e);
  }
  dotsOnLines(t, e) {
    let n = [],
      i = e.hachureGap;
    i < 0 && (i = 4 * e.strokeWidth), (i = Math.max(i, 0.1));
    let a = e.fillWeight;
    a < 0 && (a = e.strokeWidth / 2);
    for (const o of t) {
      const t = s(o) / i,
        h = Math.ceil(t) - 1,
        r = Math.atan((o[1][1] - o[0][1]) / (o[1][0] - o[0][0]));
      for (let t = 0; t < h; t++) {
        const s = i * (t + 1),
          h = s * Math.sin(r),
          c = s * Math.cos(r),
          l = [o[0][0] - c, o[0][1] + h],
          u = this.helper.randOffsetWithRange(l[0] - i / 4, l[0] + i / 4, e),
          p = this.helper.randOffsetWithRange(l[1] - i / 4, l[1] + i / 4, e),
          d = this.helper.ellipse(u, p, a, a, e);
        n = n.concat(d.ops);
      }
    }
    return { type: "fillSketch", ops: n };
  }
}
class f {
  constructor(t) {
    this.helper = t;
  }
  fillPolygon(t, e) {
    const s = c(t, e);
    return { type: "fillSketch", ops: this.dashedLine(s, e) };
  }
  dashedLine(t, e) {
    const n =
        e.dashOffset < 0
          ? e.hachureGap < 0
            ? 4 * e.strokeWidth
            : e.hachureGap
          : e.dashOffset,
      i =
        e.dashGap < 0 ? (e.hachureGap < 0 ? 4 * e.strokeWidth : e.hachureGap) : e.dashGap;
    let a = [];
    return (
      t.forEach((t) => {
        const o = s(t),
          h = Math.floor(o / (n + i)),
          r = (o + i - h * (n + i)) / 2;
        let c = t[0],
          l = t[1];
        c[0] > l[0] && ((c = t[1]), (l = t[0]));
        const u = Math.atan((l[1] - c[1]) / (l[0] - c[0]));
        for (let t = 0; t < h; t++) {
          const s = t * (n + i),
            o = s + n,
            h = [
              c[0] + s * Math.cos(u) + r * Math.cos(u),
              c[1] + s * Math.sin(u) + r * Math.sin(u),
            ],
            l = [
              c[0] + o * Math.cos(u) + r * Math.cos(u),
              c[1] + o * Math.sin(u) + r * Math.sin(u),
            ];
          a = a.concat(this.helper.doubleLineOps(h[0], h[1], l[0], l[1], e));
        }
      }),
      a
    );
  }
}
class g {
  constructor(t) {
    this.helper = t;
  }
  fillPolygon(t, e) {
    const s = e.hachureGap < 0 ? 4 * e.strokeWidth : e.hachureGap,
      n = e.zigzagOffset < 0 ? s : e.zigzagOffset,
      i = c(t, (e = Object.assign({}, e, { hachureGap: s + n })));
    return { type: "fillSketch", ops: this.zigzagLines(i, n, e) };
  }
  zigzagLines(t, e, n) {
    let i = [];
    return (
      t.forEach((t) => {
        const a = s(t),
          o = Math.round(a / (2 * e));
        let h = t[0],
          r = t[1];
        h[0] > r[0] && ((h = t[1]), (r = t[0]));
        const c = Math.atan((r[1] - h[1]) / (r[0] - h[0]));
        for (let t = 0; t < o; t++) {
          const s = 2 * t * e,
            a = 2 * (t + 1) * e,
            o = Math.sqrt(2 * Math.pow(e, 2)),
            r = [h[0] + s * Math.cos(c), h[1] + s * Math.sin(c)],
            l = [h[0] + a * Math.cos(c), h[1] + a * Math.sin(c)],
            u = [
              r[0] + o * Math.cos(c + Math.PI / 4),
              r[1] + o * Math.sin(c + Math.PI / 4),
            ];
          (i = i.concat(this.helper.doubleLineOps(r[0], r[1], u[0], u[1], n))),
            (i = i.concat(this.helper.doubleLineOps(u[0], u[1], l[0], l[1], n)));
        }
      }),
      i
    );
  }
}
const y = {};
class _ {
  constructor(t) {
    this.seed = t;
  }
  next() {
    return this.seed
      ? ((Math.pow(2, 31) - 1) & (this.seed = Math.imul(48271, this.seed))) /
          Math.pow(2, 31)
      : Math.random();
  }
}
const M = {
  randOffset: function (t, e) {
    return z(t, e);
  },
  randOffsetWithRange: function (t, e, s) {
    return C(t, e, s);
  },
  ellipse: function (t, e, s, n, i) {
    const a = P(s, n, i);
    return w(t, e, i, a).opset;
  },
  doubleLineOps: function (t, e, s, n, i) {
    return A(t, e, s, n, i);
  },
};
function x(t, e, s, n, i) {
  return { type: "path", ops: A(t, e, s, n, i) };
}
function m(t, e, s) {
  const n = (t || []).length;
  if (n > 2) {
    let i = [];
    for (let e = 0; e < n - 1; e++)
      i = i.concat(A(t[e][0], t[e][1], t[e + 1][0], t[e + 1][1], s));
    return (
      e && (i = i.concat(A(t[n - 1][0], t[n - 1][1], t[0][0], t[0][1], s))),
      { type: "path", ops: i }
    );
  }
  return 2 === n ? x(t[0][0], t[0][1], t[1][0], t[1][1], s) : { type: "path", ops: [] };
}
function k(t, e, s, n, i) {
  return (function (t, e) {
    return m(t, !0, e);
  })(
    [
      [t, e],
      [t + s, e],
      [t + s, e + n],
      [t, e + n],
    ],
    i
  );
}
function b(t, e) {
  const s = W(t, 1 * (1 + 0.2 * e.roughness), e),
    n = W(t, 1.5 * (1 + 0.22 * e.roughness), e);
  return { type: "path", ops: s.concat(n) };
}
function P(t, e, s) {
  const n = Math.sqrt(
      2 * Math.PI * Math.sqrt((Math.pow(t / 2, 2) + Math.pow(e / 2, 2)) / 2)
    ),
    i = Math.max(s.curveStepCount, (s.curveStepCount / Math.sqrt(200)) * n),
    a = (2 * Math.PI) / i;
  let o = Math.abs(t / 2),
    h = Math.abs(e / 2);
  const r = 1 - s.curveFitting;
  return (o += z(o * r, s)), (h += z(h * r, s)), { increment: a, rx: o, ry: h };
}
function w(t, e, s, n) {
  const [i, a] = D(
      n.increment,
      t,
      e,
      n.rx,
      n.ry,
      1,
      n.increment * C(0.1, C(0.4, 1, s), s),
      s
    ),
    [o] = D(n.increment, t, e, n.rx, n.ry, 1.5, 0, s),
    h = R(i, null, s),
    r = R(o, null, s);
  return { estimatedPoints: a, opset: { type: "path", ops: h.concat(r) } };
}
function v(t, e, s, n, i, a, o, h, r) {
  const c = t,
    l = e;
  let u = Math.abs(s / 2),
    p = Math.abs(n / 2);
  (u += z(0.01 * u, r)), (p += z(0.01 * p, r));
  let d = i,
    f = a;
  for (; d < 0; ) (d += 2 * Math.PI), (f += 2 * Math.PI);
  f - d > 2 * Math.PI && ((d = 0), (f = 2 * Math.PI));
  const g = (2 * Math.PI) / r.curveStepCount,
    y = Math.min(g / 2, (f - d) / 2),
    _ = I(y, c, l, u, p, d, f, 1, r),
    M = I(y, c, l, u, p, d, f, 1.5, r);
  let x = _.concat(M);
  return (
    o &&
      (h
        ? ((x = x.concat(A(c, l, c + u * Math.cos(d), l + p * Math.sin(d), r))),
          (x = x.concat(A(c, l, c + u * Math.cos(f), l + p * Math.sin(f), r))))
        : (x.push({ op: "lineTo", data: [c, l] }),
          x.push({
            op: "lineTo",
            data: [c + u * Math.cos(d), l + p * Math.sin(d)],
          }))),
    { type: "path", ops: x }
  );
}
function S(t, e) {
  const s = [];
  if (t.length) {
    const n = e.maxRandomnessOffset || 0,
      i = t.length;
    if (i > 2) {
      s.push({ op: "move", data: [t[0][0] + z(n, e), t[0][1] + z(n, e)] });
      for (let a = 1; a < i; a++)
        s.push({ op: "lineTo", data: [t[a][0] + z(n, e), t[a][1] + z(n, e)] });
    }
  }
  return { type: "fillPath", ops: s };
}
function O(t, e) {
  return (function (t, e) {
    let s = t.fillStyle || "hachure";
    if (!y[s])
      switch (s) {
        case "zigzag":
          y[s] || (y[s] = new u(e));
          break;
        case "cross-hatch":
          y[s] || (y[s] = new p(e));
          break;
        case "dots":
          y[s] || (y[s] = new d(e));
          break;
        case "dashed":
          y[s] || (y[s] = new f(e));
          break;
        case "zigzag-line":
          y[s] || (y[s] = new g(e));
          break;
        case "hachure":
        default:
          (s = "hachure"), y[s] || (y[s] = new l(e));
      }
    return y[s];
  })(e, M).fillPolygon(t, e);
}
function T(t) {
  return t.randomizer || (t.randomizer = new _(t.seed || 0)), t.randomizer.next();
}
function C(t, e, s) {
  return s.roughness * s.roughnessGain * (T(s) * (e - t) + t);
}
function z(t, e) {
  return C(-t, t, e);
}
function A(t, e, s, n, i) {
  const a = E(t, e, s, n, i, !0, !1),
    o = E(t, e, s, n, i, !0, !0);
  return a.concat(o);
}
function E(t, e, s, n, i, a, o) {
  const h = Math.pow(t - s, 2) + Math.pow(e - n, 2),
    r = Math.sqrt(h);
  i.roughnessGain = r < 200 ? 1 : r > 500 ? 0.4 : -0.0016668 * r + 1.233334;
  let c = i.maxRandomnessOffset || 0;
  c * c * 100 > h && (c = r / 10);
  const l = c / 2,
    u = 0.2 + 0.2 * T(i);
  let p = (i.bowing * i.maxRandomnessOffset * (n - e)) / 200,
    d = (i.bowing * i.maxRandomnessOffset * (t - s)) / 200;
  (p = z(p, i)), (d = z(d, i));
  const f = [],
    g = () => z(l, i),
    y = () => z(c, i);
  return (
    a &&
      (o
        ? f.push({ op: "move", data: [t + g(), e + g()] })
        : f.push({ op: "move", data: [t + z(c, i), e + z(c, i)] })),
    o
      ? f.push({
          op: "bcurveTo",
          data: [
            p + t + (s - t) * u + g(),
            d + e + (n - e) * u + g(),
            p + t + 2 * (s - t) * u + g(),
            d + e + 2 * (n - e) * u + g(),
            s + g(),
            n + g(),
          ],
        })
      : f.push({
          op: "bcurveTo",
          data: [
            p + t + (s - t) * u + y(),
            d + e + (n - e) * u + y(),
            p + t + 2 * (s - t) * u + y(),
            d + e + 2 * (n - e) * u + y(),
            s + y(),
            n + y(),
          ],
        }),
    f
  );
}
function W(t, e, s) {
  const n = [];
  n.push([t[0][0] + z(e, s), t[0][1] + z(e, s)]),
    n.push([t[0][0] + z(e, s), t[0][1] + z(e, s)]);
  for (let i = 1; i < t.length; i++)
    n.push([t[i][0] + z(e, s), t[i][1] + z(e, s)]),
      i === t.length - 1 && n.push([t[i][0] + z(e, s), t[i][1] + z(e, s)]);
  return R(n, null, s);
}
function R(t, e, s) {
  const n = t.length;
  let i = [];
  if (n > 3) {
    const a = [],
      o = 1 - s.curveTightness;
    i.push({ op: "move", data: [t[1][0], t[1][1]] });
    for (let e = 1; e + 2 < n; e++) {
      const s = t[e];
      (a[0] = [s[0], s[1]]),
        (a[1] = [
          s[0] + (o * t[e + 1][0] - o * t[e - 1][0]) / 6,
          s[1] + (o * t[e + 1][1] - o * t[e - 1][1]) / 6,
        ]),
        (a[2] = [
          t[e + 1][0] + (o * t[e][0] - o * t[e + 2][0]) / 6,
          t[e + 1][1] + (o * t[e][1] - o * t[e + 2][1]) / 6,
        ]),
        (a[3] = [t[e + 1][0], t[e + 1][1]]),
        i.push({
          op: "bcurveTo",
          data: [a[1][0], a[1][1], a[2][0], a[2][1], a[3][0], a[3][1]],
        });
    }
    if (e && 2 === e.length) {
      const t = s.maxRandomnessOffset;
      i.push({ op: "lineTo", data: [e[0] + z(t, s), e[1] + z(t, s)] });
    }
  } else
    3 === n
      ? (i.push({ op: "move", data: [t[1][0], t[1][1]] }),
        i.push({
          op: "bcurveTo",
          data: [t[1][0], t[1][1], t[2][0], t[2][1], t[2][0], t[2][1]],
        }))
      : 2 === n && (i = i.concat(A(t[0][0], t[0][1], t[1][0], t[1][1], s)));
  return i;
}
function D(t, e, s, n, i, a, o, h) {
  const r = [],
    c = [],
    l = z(0.5, h) - Math.PI / 2;
  c.push([
    z(a, h) + e + 0.9 * n * Math.cos(l - t),
    z(a, h) + s + 0.9 * i * Math.sin(l - t),
  ]);
  for (let o = l; o < 2 * Math.PI + l - 0.01; o += t) {
    const t = [z(a, h) + e + n * Math.cos(o), z(a, h) + s + i * Math.sin(o)];
    r.push(t), c.push(t);
  }
  return (
    c.push([
      z(a, h) + e + n * Math.cos(l + 2 * Math.PI + 0.5 * o),
      z(a, h) + s + i * Math.sin(l + 2 * Math.PI + 0.5 * o),
    ]),
    c.push([
      z(a, h) + e + 0.98 * n * Math.cos(l + o),
      z(a, h) + s + 0.98 * i * Math.sin(l + o),
    ]),
    c.push([
      z(a, h) + e + 0.9 * n * Math.cos(l + 0.5 * o),
      z(a, h) + s + 0.9 * i * Math.sin(l + 0.5 * o),
    ]),
    [c, r]
  );
}
function I(t, e, s, n, i, a, o, h, r) {
  const c = a + z(0.1, r),
    l = [];
  l.push([
    z(h, r) + e + 0.9 * n * Math.cos(c - t),
    z(h, r) + s + 0.9 * i * Math.sin(c - t),
  ]);
  for (let a = c; a <= o; a += t)
    l.push([z(h, r) + e + n * Math.cos(a), z(h, r) + s + i * Math.sin(a)]);
  return (
    l.push([e + n * Math.cos(o), s + i * Math.sin(o)]),
    l.push([e + n * Math.cos(o), s + i * Math.sin(o)]),
    R(l, null, r)
  );
}
function q(t, e, s, n, i, a, o, h) {
  const r = [],
    c = [h.maxRandomnessOffset || 1, (h.maxRandomnessOffset || 1) + 0.5];
  let l = [0, 0];
  for (let u = 0; u < 2; u++)
    0 === u
      ? r.push({ op: "move", data: [o.x, o.y] })
      : r.push({ op: "move", data: [o.x + z(c[0], h), o.y + z(c[0], h)] }),
      (l = [i + z(c[u], h), a + z(c[u], h)]),
      r.push({
        op: "bcurveTo",
        data: [
          t + z(c[u], h),
          e + z(c[u], h),
          s + z(c[u], h),
          n + z(c[u], h),
          l[0],
          l[1],
        ],
      });
  return o.setPosition(l[0], l[1]), r;
}
function $(t, e, s, n) {
  let i = [];
  switch (e.key) {
    case "M":
    case "m": {
      const s = "m" === e.key;
      if (e.data.length >= 2) {
        let a = +e.data[0],
          o = +e.data[1];
        s && ((a += t.x), (o += t.y));
        const h = 1 * (n.maxRandomnessOffset || 0);
        (a += z(h, n)),
          (o += z(h, n)),
          t.setPosition(a, o),
          i.push({ op: "move", data: [a, o] });
      }
      break;
    }
    case "L":
    case "l": {
      const s = "l" === e.key;
      if (e.data.length >= 2) {
        let a = +e.data[0],
          o = +e.data[1];
        s && ((a += t.x), (o += t.y)),
          (i = i.concat(A(t.x, t.y, a, o, n))),
          t.setPosition(a, o);
      }
      break;
    }
    case "H":
    case "h": {
      const s = "h" === e.key;
      if (e.data.length) {
        let a = +e.data[0];
        s && (a += t.x), (i = i.concat(A(t.x, t.y, a, t.y, n))), t.setPosition(a, t.y);
      }
      break;
    }
    case "V":
    case "v": {
      const s = "v" === e.key;
      if (e.data.length) {
        let a = +e.data[0];
        s && (a += t.y), (i = i.concat(A(t.x, t.y, t.x, a, n))), t.setPosition(t.x, a);
      }
      break;
    }
    case "Z":
    case "z":
      t.first &&
        ((i = i.concat(A(t.x, t.y, t.first[0], t.first[1], n))),
        t.setPosition(t.first[0], t.first[1]),
        (t.first = null));
      break;
    case "C":
    case "c": {
      const s = "c" === e.key;
      if (e.data.length >= 6) {
        let a = +e.data[0],
          o = +e.data[1],
          h = +e.data[2],
          r = +e.data[3],
          c = +e.data[4],
          l = +e.data[5];
        s && ((a += t.x), (h += t.x), (c += t.x), (o += t.y), (r += t.y), (l += t.y));
        const u = q(a, o, h, r, c, l, t, n);
        (i = i.concat(u)), (t.bezierReflectionPoint = [c + (c - h), l + (l - r)]);
      }
      break;
    }
    case "S":
    case "s": {
      const a = "s" === e.key;
      if (e.data.length >= 4) {
        let o = +e.data[0],
          h = +e.data[1],
          r = +e.data[2],
          c = +e.data[3];
        a && ((o += t.x), (r += t.x), (h += t.y), (c += t.y));
        let l = o,
          u = h;
        const p = s ? s.key : "";
        let d = null;
        ("c" !== p && "C" !== p && "s" !== p && "S" !== p) ||
          (d = t.bezierReflectionPoint),
          d && ((l = d[0]), (u = d[1]));
        const f = q(l, u, o, h, r, c, t, n);
        (i = i.concat(f)), (t.bezierReflectionPoint = [r + (r - o), c + (c - h)]);
      }
      break;
    }
    case "Q":
    case "q": {
      const s = "q" === e.key;
      if (e.data.length >= 4) {
        let a = +e.data[0],
          o = +e.data[1],
          h = +e.data[2],
          r = +e.data[3];
        s && ((a += t.x), (h += t.x), (o += t.y), (r += t.y));
        const c = 1 * (1 + 0.2 * n.roughness),
          l = 1.5 * (1 + 0.22 * n.roughness);
        i.push({ op: "move", data: [t.x + z(c, n), t.y + z(c, n)] });
        let u = [h + z(c, n), r + z(c, n)];
        i.push({
          op: "qcurveTo",
          data: [a + z(c, n), o + z(c, n), u[0], u[1]],
        }),
          i.push({ op: "move", data: [t.x + z(l, n), t.y + z(l, n)] }),
          (u = [h + z(l, n), r + z(l, n)]),
          i.push({
            op: "qcurveTo",
            data: [a + z(l, n), o + z(l, n), u[0], u[1]],
          }),
          t.setPosition(u[0], u[1]),
          (t.quadReflectionPoint = [h + (h - a), r + (r - o)]);
      }
      break;
    }
    case "T":
    case "t": {
      const a = "t" === e.key;
      if (e.data.length >= 2) {
        let o = +e.data[0],
          h = +e.data[1];
        a && ((o += t.x), (h += t.y));
        let r = o,
          c = h;
        const l = s ? s.key : "";
        let u = null;
        ("q" !== l && "Q" !== l && "t" !== l && "T" !== l) || (u = t.quadReflectionPoint),
          u && ((r = u[0]), (c = u[1]));
        const p = 1 * (1 + 0.2 * n.roughness),
          d = 1.5 * (1 + 0.22 * n.roughness);
        i.push({ op: "move", data: [t.x + z(p, n), t.y + z(p, n)] });
        let f = [o + z(p, n), h + z(p, n)];
        i.push({
          op: "qcurveTo",
          data: [r + z(p, n), c + z(p, n), f[0], f[1]],
        }),
          i.push({ op: "move", data: [t.x + z(d, n), t.y + z(d, n)] }),
          (f = [o + z(d, n), h + z(d, n)]),
          i.push({
            op: "qcurveTo",
            data: [r + z(d, n), c + z(d, n), f[0], f[1]],
          }),
          t.setPosition(f[0], f[1]),
          (t.quadReflectionPoint = [o + (o - r), h + (h - c)]);
      }
      break;
    }
    case "A":
    case "a": {
      const s = "a" === e.key;
      if (e.data.length >= 7) {
        const a = +e.data[0],
          o = +e.data[1],
          r = +e.data[2],
          c = +e.data[3],
          l = +e.data[4];
        let u = +e.data[5],
          p = +e.data[6];
        if ((s && ((u += t.x), (p += t.y)), u === t.x && p === t.y)) break;
        if (0 === a || 0 === o) (i = i.concat(A(t.x, t.y, u, p, n))), t.setPosition(u, p);
        else
          for (let e = 0; e < 1; e++) {
            const e = new h([t.x, t.y], [u, p], [a, o], r, !!c, !!l);
            let s = e.getNextSegment();
            for (; s; ) {
              const a = q(s.cp1[0], s.cp1[1], s.cp2[0], s.cp2[1], s.to[0], s.to[1], t, n);
              (i = i.concat(a)), (s = e.getNextSegment());
            }
          }
      }
      break;
    }
  }
  return i;
}
const N = "undefined" != typeof self,
  L = "none";
class B {
  constructor(t, e) {
    (this.defaultOptions = {
      maxRandomnessOffset: 2,
      roughness: 1,
      bowing: 1,
      stroke: "#000",
      strokeWidth: 1,
      curveTightness: 0,
      curveFitting: 0.95,
      curveStepCount: 9,
      fillStyle: "hachure",
      fillWeight: -1,
      hachureAngle: -41,
      hachureGap: -1,
      dashOffset: -1,
      dashGap: -1,
      zigzagOffset: -1,
      seed: 0,
      roughnessGain: 1,
    }),
      (this.config = t || {}),
      (this.surface = e),
      this.config.options && (this.defaultOptions = this._options(this.config.options));
  }
  static newSeed() {
    return Math.floor(Math.random() * Math.pow(2, 31));
  }
  _options(t) {
    return t ? Object.assign({}, this.defaultOptions, t) : this.defaultOptions;
  }
  _drawable(t, e, s) {
    return { shape: t, sets: e || [], options: s || this.defaultOptions };
  }
  line(t, e, s, n, i) {
    const a = this._options(i);
    return this._drawable("line", [x(t, e, s, n, a)], a);
  }
  rectangle(t, e, s, n, i) {
    const a = this._options(i),
      o = [],
      h = k(t, e, s, n, a);
    if (a.fill) {
      const i = [
        [t, e],
        [t + s, e],
        [t + s, e + n],
        [t, e + n],
      ];
      "solid" === a.fillStyle ? o.push(S(i, a)) : o.push(O(i, a));
    }
    return a.stroke !== L && o.push(h), this._drawable("rectangle", o, a);
  }
  ellipse(t, e, s, n, i) {
    const a = this._options(i),
      o = [],
      h = P(s, n, a),
      r = w(t, e, a, h);
    if (a.fill)
      if ("solid" === a.fillStyle) {
        const s = w(t, e, a, h).opset;
        (s.type = "fillPath"), o.push(s);
      } else o.push(O(r.estimatedPoints, a));
    return a.stroke !== L && o.push(r.opset), this._drawable("ellipse", o, a);
  }
  circle(t, e, s, n) {
    const i = this.ellipse(t, e, s, s, n);
    return (i.shape = "circle"), i;
  }
  linearPath(t, e) {
    const s = this._options(e);
    return this._drawable("linearPath", [m(t, !1, s)], s);
  }
  arc(t, e, s, n, i, a, o = !1, h) {
    const r = this._options(h),
      c = [],
      l = v(t, e, s, n, i, a, o, !0, r);
    if (o && r.fill)
      if ("solid" === r.fillStyle) {
        const o = v(t, e, s, n, i, a, !0, !1, r);
        (o.type = "fillPath"), c.push(o);
      } else
        c.push(
          (function (t, e, s, n, i, a, o) {
            const h = t,
              r = e;
            let c = Math.abs(s / 2),
              l = Math.abs(n / 2);
            (c += z(0.01 * c, o)), (l += z(0.01 * l, o));
            let u = i,
              p = a;
            for (; u < 0; ) (u += 2 * Math.PI), (p += 2 * Math.PI);
            p - u > 2 * Math.PI && ((u = 0), (p = 2 * Math.PI));
            const d = (p - u) / o.curveStepCount,
              f = [];
            for (let t = u; t <= p; t += d)
              f.push([h + c * Math.cos(t), r + l * Math.sin(t)]);
            return (
              f.push([h + c * Math.cos(p), r + l * Math.sin(p)]), f.push([h, r]), O(f, o)
            );
          })(t, e, s, n, i, a, r)
        );
    return r.stroke !== L && c.push(l), this._drawable("arc", c, r);
  }
  curve(t, e) {
    const s = this._options(e);
    return this._drawable("curve", [b(t, s)], s);
  }
  polygon(t, e) {
    const s = this._options(e),
      n = [],
      i = m(t, !0, s);
    return (
      s.fill && ("solid" === s.fillStyle ? n.push(S(t, s)) : n.push(O(t, s))),
      s.stroke !== L && n.push(i),
      this._drawable("polygon", n, s)
    );
  }
  path(t, e) {
    const s = this._options(e),
      n = [];
    if (!t) return this._drawable("path", n, s);
    const i = (function (t, e) {
      t = (t || "").replace(/\n/g, " ").replace(/(-\s)/g, "-").replace("/(ss)/g", " ");
      let s = new o(t);
      if (e.simplification) {
        const t = new r(s.linearPoints, s.closed).fit(e.simplification);
        s = new o(t);
      }
      let n = [];
      const i = s.segments || [];
      for (let t = 0; t < i.length; t++) {
        const a = $(s, i[t], t > 0 ? i[t - 1] : null, e);
        a && a.length && (n = n.concat(a));
      }
      return { type: "path", ops: n };
    })(t, s);
    if (s.fill)
      if ("solid" === s.fillStyle) {
        const e = { type: "path2Dfill", path: t, ops: [] };
        n.push(e);
      } else {
        const e = this.computePathSize(t),
          i = O(
            [
              [0, 0],
              [e[0], 0],
              [e[0], e[1]],
              [0, e[1]],
            ],
            s
          );
        (i.type = "path2Dpattern"), (i.size = e), (i.path = t), n.push(i);
      }
    return s.stroke !== L && n.push(i), this._drawable("path", n, s);
  }
  computePathSize(e) {
    let s = [0, 0];
    if (N && self.document)
      try {
        const n = self.document.createElementNS(t, "svg");
        n.setAttribute("width", "0"), n.setAttribute("height", "0");
        const i = self.document.createElementNS(t, "path");
        i.setAttribute("d", e), n.appendChild(i), self.document.body.appendChild(n);
        const a = i.getBBox();
        a && ((s[0] = a.width || 0), (s[1] = a.height || 0)),
          self.document.body.removeChild(n);
      } catch (t) {}
    const n = this.getCanvasSize();
    return s[0] * s[1] || (s = n), s;
  }
  getCanvasSize() {
    const t = (t) =>
      t && "object" == typeof t && t.baseVal && t.baseVal.value
        ? t.baseVal.value
        : t || 100;
    return this.surface ? [t(this.surface.width), t(this.surface.height)] : [100, 100];
  }
  opsToPath(t) {
    let e = "";
    for (const s of t.ops) {
      const t = s.data;
      switch (s.op) {
        case "move":
          e += `M${t[0]} ${t[1]} `;
          break;
        case "bcurveTo":
          e += `C${t[0]} ${t[1]}, ${t[2]} ${t[3]}, ${t[4]} ${t[5]} `;
          break;
        case "qcurveTo":
          e += `Q${t[0]} ${t[1]}, ${t[2]} ${t[3]} `;
          break;
        case "lineTo":
          e += `L${t[0]} ${t[1]} `;
      }
    }
    return e.trim();
  }
  toPaths(t) {
    const e = t.sets || [],
      s = t.options || this.defaultOptions,
      n = [];
    for (const t of e) {
      let e = null;
      switch (t.type) {
        case "path":
          e = {
            d: this.opsToPath(t),
            stroke: s.stroke,
            strokeWidth: s.strokeWidth,
            fill: L,
          };
          break;
        case "fillPath":
          e = {
            d: this.opsToPath(t),
            stroke: L,
            strokeWidth: 0,
            fill: s.fill || L,
          };
          break;
        case "fillSketch":
          e = this.fillSketch(t, s);
          break;
        case "path2Dfill":
          e = { d: t.path || "", stroke: L, strokeWidth: 0, fill: s.fill || L };
          break;
        case "path2Dpattern": {
          const n = t.size,
            i = {
              x: 0,
              y: 0,
              width: 1,
              height: 1,
              viewBox: `0 0 ${Math.round(n[0])} ${Math.round(n[1])}`,
              patternUnits: "objectBoundingBox",
              path: this.fillSketch(t, s),
            };
          e = { d: t.path, stroke: L, strokeWidth: 0, pattern: i };
          break;
        }
      }
      e && n.push(e);
    }
    return n;
  }
  fillSketch(t, e) {
    let s = e.fillWeight;
    return (
      s < 0 && (s = e.strokeWidth / 2),
      { d: this.opsToPath(t), stroke: e.fill || L, strokeWidth: s, fill: L }
    );
  }
}
const G = "undefined" != typeof document;
class V {
  constructor(t, e) {
    (this.canvas = t),
      (this.ctx = this.canvas.getContext("2d")),
      (this.gen = new B(e, this.canvas));
  }
  draw(t) {
    const e = t.sets || [],
      s = t.options || this.getDefaultOptions(),
      n = this.ctx;
    for (const t of e)
      switch (t.type) {
        case "path":
          n.save(),
            (n.strokeStyle = "none" === s.stroke ? "transparent" : s.stroke),
            (n.lineWidth = s.strokeWidth),
            this._drawToContext(n, t),
            n.restore();
          break;
        case "fillPath":
          n.save(), (n.fillStyle = s.fill || ""), this._drawToContext(n, t), n.restore();
          break;
        case "fillSketch":
          this.fillSketch(n, t, s);
          break;
        case "path2Dfill": {
          this.ctx.save(), (this.ctx.fillStyle = s.fill || "");
          const e = new Path2D(t.path);
          this.ctx.fill(e), this.ctx.restore();
          break;
        }
        case "path2Dpattern": {
          const e = this.canvas.ownerDocument || (G && document);
          if (e) {
            const n = t.size,
              i = e.createElement("canvas"),
              a = i.getContext("2d"),
              o = this.computeBBox(t.path);
            o && (o.width || o.height)
              ? ((i.width = this.canvas.width),
                (i.height = this.canvas.height),
                a.translate(o.x || 0, o.y || 0))
              : ((i.width = n[0]), (i.height = n[1])),
              this.fillSketch(a, t, s),
              this.ctx.save(),
              (this.ctx.fillStyle = this.ctx.createPattern(i, "repeat"));
            const h = new Path2D(t.path);
            this.ctx.fill(h), this.ctx.restore();
          } else console.error("Pattern fill fail: No defs");
          break;
        }
      }
  }
  computeBBox(e) {
    if (G)
      try {
        const s = document.createElementNS(t, "svg");
        s.setAttribute("width", "0"), s.setAttribute("height", "0");
        const n = self.document.createElementNS(t, "path");
        n.setAttribute("d", e), s.appendChild(n), document.body.appendChild(s);
        const i = n.getBBox();
        return document.body.removeChild(s), i;
      } catch (t) {}
    return null;
  }
  fillSketch(t, e, s) {
    let n = s.fillWeight;
    n < 0 && (n = s.strokeWidth / 2),
      t.save(),
      (t.strokeStyle = s.fill || ""),
      (t.lineWidth = n),
      this._drawToContext(t, e),
      t.restore();
  }
  _drawToContext(t, e) {
    t.beginPath();
    for (const s of e.ops) {
      const e = s.data;
      switch (s.op) {
        case "move":
          t.moveTo(e[0], e[1]);
          break;
        case "bcurveTo":
          t.bezierCurveTo(e[0], e[1], e[2], e[3], e[4], e[5]);
          break;
        case "qcurveTo":
          t.quadraticCurveTo(e[0], e[1], e[2], e[3]);
          break;
        case "lineTo":
          t.lineTo(e[0], e[1]);
      }
    }
    "fillPath" === e.type ? t.fill() : t.stroke();
  }
  get generator() {
    return this.gen;
  }
  getDefaultOptions() {
    return this.gen.defaultOptions;
  }
  line(t, e, s, n, i) {
    const a = this.gen.line(t, e, s, n, i);
    return this.draw(a), a;
  }
  rectangle(t, e, s, n, i) {
    const a = this.gen.rectangle(t, e, s, n, i);
    return this.draw(a), a;
  }
  ellipse(t, e, s, n, i) {
    const a = this.gen.ellipse(t, e, s, n, i);
    return this.draw(a), a;
  }
  circle(t, e, s, n) {
    const i = this.gen.circle(t, e, s, n);
    return this.draw(i), i;
  }
  linearPath(t, e) {
    const s = this.gen.linearPath(t, e);
    return this.draw(s), s;
  }
  polygon(t, e) {
    const s = this.gen.polygon(t, e);
    return this.draw(s), s;
  }
  arc(t, e, s, n, i, a, o = !1, h) {
    const r = this.gen.arc(t, e, s, n, i, a, o, h);
    return this.draw(r), r;
  }
  curve(t, e) {
    const s = this.gen.curve(t, e);
    return this.draw(s), s;
  }
  path(t, e) {
    const s = this.gen.path(t, e);
    return this.draw(s), s;
  }
}
const j = "undefined" != typeof document;
class Q {
  constructor(t, e) {
    (this.svg = t), (this.gen = new B(e, this.svg));
  }
  get defs() {
    const e = this.svg.ownerDocument || (j && document);
    if (e && !this._defs) {
      const s = e.createElementNS(t, "defs");
      this.svg.firstChild
        ? this.svg.insertBefore(s, this.svg.firstChild)
        : this.svg.appendChild(s),
        (this._defs = s);
    }
    return this._defs || null;
  }
  draw(e) {
    const s = e.sets || [],
      n = e.options || this.getDefaultOptions(),
      i = this.svg.ownerDocument || window.document,
      a = i.createElementNS(t, "g");
    for (const e of s) {
      let s = null;
      switch (e.type) {
        case "path":
          (s = i.createElementNS(t, "path")),
            s.setAttribute("d", this.opsToPath(e)),
            (s.style.stroke = n.stroke),
            (s.style.strokeWidth = n.strokeWidth + ""),
            (s.style.fill = "none");
          break;
        case "fillPath":
          (s = i.createElementNS(t, "path")),
            s.setAttribute("d", this.opsToPath(e)),
            (s.style.stroke = "none"),
            (s.style.strokeWidth = "0"),
            (s.style.fill = n.fill || "");
          break;
        case "fillSketch":
          s = this.fillSketch(i, e, n);
          break;
        case "path2Dfill":
          (s = i.createElementNS(t, "path")),
            s.setAttribute("d", e.path || ""),
            (s.style.stroke = "none"),
            (s.style.strokeWidth = "0"),
            (s.style.fill = n.fill || "");
          break;
        case "path2Dpattern":
          if (this.defs) {
            const a = e.size,
              o = i.createElementNS(t, "pattern"),
              h = `rough-${Math.floor(
                Math.random() * (Number.MAX_SAFE_INTEGER || 999999)
              )}`;
            o.setAttribute("id", h),
              o.setAttribute("x", "0"),
              o.setAttribute("y", "0"),
              o.setAttribute("width", "1"),
              o.setAttribute("height", "1"),
              o.setAttribute("height", "1"),
              o.setAttribute("viewBox", `0 0 ${Math.round(a[0])} ${Math.round(a[1])}`),
              o.setAttribute("patternUnits", "objectBoundingBox");
            const r = this.fillSketch(i, e, n);
            o.appendChild(r),
              this.defs.appendChild(o),
              (s = i.createElementNS(t, "path")),
              s.setAttribute("d", e.path || ""),
              (s.style.stroke = "none"),
              (s.style.strokeWidth = "0"),
              (s.style.fill = `url(#${h})`);
          } else console.error("Pattern fill fail: No defs");
      }
      s && a.appendChild(s);
    }
    return a;
  }
  fillSketch(e, s, n) {
    let i = n.fillWeight;
    i < 0 && (i = n.strokeWidth / 2);
    const a = e.createElementNS(t, "path");
    return (
      a.setAttribute("d", this.opsToPath(s)),
      (a.style.stroke = n.fill || ""),
      (a.style.strokeWidth = i + ""),
      (a.style.fill = "none"),
      a
    );
  }
  get generator() {
    return this.gen;
  }
  getDefaultOptions() {
    return this.gen.defaultOptions;
  }
  opsToPath(t) {
    return this.gen.opsToPath(t);
  }
  line(t, e, s, n, i) {
    const a = this.gen.line(t, e, s, n, i);
    return this.draw(a);
  }
  rectangle(t, e, s, n, i) {
    const a = this.gen.rectangle(t, e, s, n, i);
    return this.draw(a);
  }
  ellipse(t, e, s, n, i) {
    const a = this.gen.ellipse(t, e, s, n, i);
    return this.draw(a);
  }
  circle(t, e, s, n) {
    const i = this.gen.circle(t, e, s, n);
    return this.draw(i);
  }
  linearPath(t, e) {
    const s = this.gen.linearPath(t, e);
    return this.draw(s);
  }
  polygon(t, e) {
    const s = this.gen.polygon(t, e);
    return this.draw(s);
  }
  arc(t, e, s, n, i, a, o = !1, h) {
    const r = this.gen.arc(t, e, s, n, i, a, o, h);
    return this.draw(r);
  }
  curve(t, e) {
    const s = this.gen.curve(t, e);
    return this.draw(s);
  }
  path(t, e) {
    const s = this.gen.path(t, e);
    return this.draw(s);
  }
}
var rough = {
  canvas: (t, e) => new V(t, e),
  svg: (t, e) => new Q(t, e),
  generator: (t, e) => new B(t, e),
  newSeed: () => B.newSeed(),
};
