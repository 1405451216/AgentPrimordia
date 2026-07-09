/**
 * 统计显著性检验 — Prompt A/B 平台的数学内核（对应 evolution 计划 T2-2）。
 *
 * 纯函数实现，零运行时依赖，可单元测试：
 * - Welch's t-test（两组均值差异，不假设方差齐性）
 * - 置信区间 / 标准误
 * - Cohen's d 效应量
 * - 卡方独立性检验（分类变量）
 * - 两比例 Z 检验
 *
 * 注：t 分布与正态分布的 CDF 通过正则化不完全 Beta 函数 / 误差函数近似实现，
 * 数值方法与 Numerical Recipes 一致，无需第三方统计库。
 */

export interface DatasetStats {
  n: number;
  mean: number;
  variance: number;
  stddev: number;
  sem: number;
}

export interface SignificanceResult {
  /** 检验统计量（t 或 z 或 chi2） */
  statistic: number;
  /** 自由度（t 检验） */
  df?: number;
  /** 双侧 p 值 */
  pValue: number;
  /** 在给定置信水平下是否显著 */
  isSignificant: boolean;
  /** 效应量（Cohen's d 或 phi），无则为 undefined */
  effectSize?: number;
}

/** 计算样本统计量 */
export function describe(values: number[]): DatasetStats {
  const n = values.length;
  if (n === 0) {
    return { n: 0, mean: 0, variance: 0, stddev: 0, sem: 0 };
  }
  const mean = values.reduce((a, b) => a + b, 0) / n;
  const variance = n > 1
    ? values.reduce((a, b) => a + (b - mean) ** 2, 0) / (n - 1)
    : 0;
  const stddev = Math.sqrt(variance);
  return { n, mean, variance, stddev, sem: stddev / Math.sqrt(n) };
}

/** 标准正态 CDF（通过 erf 近似） */
export function normalCDF(x: number): number {
  return 0.5 * (1 + erf(x / Math.SQRT2));
}

/** 误差函数（Abramowitz & Stegun 7.1.26 近似） */
export function erf(x: number): number {
  const sign = x < 0 ? -1 : 1;
  const ax = Math.abs(x);
  const t = 1 / (1 + 0.3275911 * ax);
  const y = 1 - (((((1.061405429 * t - 1.453152027) * t) + 1.421413741) * t - 0.284496736) * t + 0.254829592) * t * Math.exp(-ax * ax);
  return sign * y;
}

/** 均值的置信区间 [lo, hi]（默认 95%） */
export function confidenceInterval(
  mean: number,
  stddev: number,
  n: number,
  confidence = 0.95,
): [number, number] {
  if (n <= 1) return [mean, mean];
  const z = zForConfidence(confidence);
  const margin = (z * stddev) / Math.sqrt(n);
  return [mean - margin, mean + margin];
}

function zForConfidence(confidence: number): number {
  const alpha = 1 - confidence;
  // 反标准正态 CDF 近似（Acklam 算法简化版）
  return -normInvCDF(alpha / 2);
}

/** 标准正态分位数（逆 CDF）近似 */
export function normInvCDF(p: number): number {
  if (p <= 0) return -Infinity;
  if (p >= 1) return Infinity;
  const a = [-3.969683028665376e1, 2.209460984245205e2, -2.759285104469687e2, 1.38357751867269e2, -3.066479806614716e1, 2.506628277459239];
  const b = [-5.447609879822406e1, 1.615858368580409e2, -1.556989798598866e2, 6.680131188771972e1, -1.328068155288572e1];
  const c = [-7.784894002430293e-3, -3.223964580411365e-1, -2.400758277161838, -2.549732539343734, 4.374664141464968, 2.938163982698783];
  const d = [7.784695709041462e-3, 3.224671290700398e-1, 2.445134137142996, 3.754408661907416];
  const plow = 0.02425;
  const phigh = 1 - plow;
  let q: number, r: number;
  if (p < plow) {
    q = Math.sqrt(-2 * Math.log(p));
    return (((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) /
      ((((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1);
  }
  if (p <= phigh) {
    q = p - 0.5;
    r = q * q;
    return (((((a[0] * r + a[1]) * r + a[2]) * r + a[3]) * r + a[4]) * r + a[5]) * q /
      (((((b[0] * r + b[1]) * r + b[2]) * r + b[3]) * r + b[4]) * r + 1);
  }
  q = Math.sqrt(-2 * Math.log(1 - p));
  return -(((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) /
    ((((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1);
}

/** Cohen's d 效应量（两组） */
export function cohensD(a: number[], b: number[]): number {
  const sa = describe(a);
  const sb = describe(b);
  const pooled = Math.sqrt(
    ((sa.n - 1) * sa.variance + (sb.n - 1) * sb.variance) / Math.max(1, sa.n + sb.n - 2),
  );
  if (pooled === 0) return 0;
  return (sa.mean - sb.mean) / pooled;
}

/** 正则化不完全 Beta 函数 I_x(a,b) */
export function betai(a: number, b: number, x: number): number {
  if (x <= 0) return 0;
  if (x >= 1) return 1;
  const lbeta = logBeta(a, b);
  const front = Math.exp(Math.log(x) * a + Math.log(1 - x) * b - lbeta) / a;
  let bt: number;
  if (x < (a + 1) / (a + b + 2)) {
    bt = front * betacf(a, b, x);
    return bt;
  }
  bt = front * betacf(b, a, 1 - x);
  return 1 - bt;
}

function logBeta(a: number, b: number): number {
  return logGamma(a) + logGamma(b) - logGamma(a + b);
}

function logGamma(x: number): number {
  const g = 7;
  const c = [
    0.99999999999980993, 676.5203681218851, -1259.1392167224028,
    771.32342877765313, -176.61502916214059, 12.507343278686905,
    -0.13857109526572012, 9.9843695780195716e-6, 1.5056327351493116e-7,
  ];
  if (x < 0.5) {
    return Math.log(Math.PI / Math.sin(Math.PI * x)) - logGamma(1 - x);
  }
  x -= 1;
  let a = c[0];
  const t = x + g + 0.5;
  for (let i = 1; i < g + 2; i++) a += c[i] / (x + i);
  return 0.5 * Math.log(2 * Math.PI) + (x + 0.5) * Math.log(t) - t + Math.log(a);
}

function betacf(a: number, b: number, x: number): number {
  const maxIter = 200;
  const eps = 3e-12;
  const qab = a + b;
  const qap = a + 1;
  const qam = a - 1;
  let c = 1;
  let d = 1 - (qab * x) / qap;
  if (Math.abs(d) < 1e-30) d = 1e-30;
  d = 1 / d;
  let h = d;
  for (let m = 1; m <= maxIter; m++) {
    const m2 = 2 * m;
    let aa = (m * (b - m) * x) / ((qam + m2) * (a + m2));
    d = 1 + aa * d;
    if (Math.abs(d) < 1e-30) d = 1e-30;
    c = 1 + aa / c;
    if (Math.abs(c) < 1e-30) c = 1e-30;
    d = 1 / d;
    h *= d * c;
    aa = (-(a + m) * (qab + m) * x) / ((a + m2) * (qap + m2));
    d = 1 + aa * d;
    if (Math.abs(d) < 1e-30) d = 1e-30;
    c = 1 + aa / c;
    if (Math.abs(c) < 1e-30) c = 1e-30;
    d = 1 / d;
    const del = d * c;
    h *= del;
    if (Math.abs(del - 1) < eps) break;
  }
  return h;
}

/** 学生 t 分布 CDF */
export function studentTCDF(t: number, df: number): number {
  const x = df / (df + t * t);
  const ib = 0.5 * betai(df / 2, 0.5, x);
  return t > 0 ? 1 - ib : ib;
}

/** Welch's t-test（两组独立样本，不假设方差齐性） */
export function welchTTest(
  a: number[],
  b: number[],
  confidence = 0.95,
): SignificanceResult & { meanA: number; meanB: number; ciA: [number, number]; ciB: [number, number] } {
  const sa = describe(a);
  const sb = describe(b);
  const alpha = 1 - confidence;

  const varA = sa.variance / Math.max(1, sa.n);
  const varB = sb.variance / Math.max(1, sb.n);
  const denom = Math.sqrt(varA + varB);

  if (denom === 0 || sa.n === 0 || sb.n === 0) {
    return {
      statistic: 0, df: sa.n + sb.n - 2, pValue: 1, isSignificant: false,
      effectSize: 0, meanA: sa.mean, meanB: sb.mean,
      ciA: [sa.mean, sa.mean], ciB: [sb.mean, sb.mean],
    };
  }

  const t = (sa.mean - sb.mean) / denom;
  const df = Math.floor(
    (varA + varB) ** 2 /
    ((varA ** 2) / Math.max(1, sa.n - 1) + (varB ** 2) / Math.max(1, sb.n - 1)),
  );

  const pValue = 2 * (1 - studentTCDF(Math.abs(t), df));
  const effect = cohensD(a, b);

  return {
    statistic: t,
    df,
    pValue,
    isSignificant: pValue < alpha,
    effectSize: effect,
    meanA: sa.mean,
    meanB: sb.mean,
    ciA: confidenceInterval(sa.mean, sa.stddev, sa.n, confidence),
    ciB: confidenceInterval(sb.mean, sb.stddev, sb.n, confidence),
  };
}

/** 卡方独立性检验（2 维列联表） */
export function chiSquareContingency(table: number[][]): SignificanceResult {
  const rows = table.length;
  const cols = table[0]?.length ?? 0;
  const rowSum = new Array(rows).fill(0);
  const colSum = new Array(cols).fill(0);
  let total = 0;
  for (let i = 0; i < rows; i++) {
    for (let j = 0; j < cols; j++) {
      const v = table[i][j] ?? 0;
      rowSum[i] += v;
      colSum[j] += v;
      total += v;
    }
  }
  if (total === 0) return { statistic: 0, df: (rows - 1) * (cols - 1), pValue: 1, isSignificant: false };

  let chi2 = 0;
  for (let i = 0; i < rows; i++) {
    for (let j = 0; j < cols; j++) {
      const expected = (rowSum[i] * colSum[j]) / total;
      if (expected > 0) {
        const diff = (table[i][j] ?? 0) - expected;
        chi2 += (diff * diff) / expected;
      }
    }
  }
  const df = (rows - 1) * (cols - 1);
  const pValue = 1 - chiSquareCDF(chi2, df);
  // 卡方效应量 phi（sqrt(chi2 / n)）
  const effectSize = Math.sqrt(chi2 / total);
  return { statistic: chi2, df, pValue, isSignificant: pValue < 0.05, effectSize };
}

function chiSquareCDF(x: number, df: number): number {
  // 用正则化下不完全 Gamma 的特例：chi2 分布 = Gamma(df/2, 2)
  return lowerIncompleteGamma(df / 2, x / 2);
}

function lowerIncompleteGamma(s: number, x: number): number {
  // 序列展开（对小 x）+ 连分数（对大 x），这里用数值稳定的级数和
  if (x < 0) return 0;
  if (x === 0) return 0;
  if (x < s + 1) {
    let sum = Math.exp(-x);
    let term = sum;
    for (let n = 1; n < 1000; n++) {
      term *= x / (s + n);
      sum += term;
      if (term < sum * 1e-12) break;
    }
    return sum;
  }
  return 1 - gammq(s, x);
}

function gammq(s: number, x: number): number {
  // 连分数表示的上不完全 Gamma（简化实现）
  const maxIter = 200;
  const eps = 1e-12;
  let b = x + 1 - s;
  let c = 1 / 1e-30;
  let d = 1 / b;
  let h = d;
  for (let i = 1; i <= maxIter; i++) {
    const an = -i * (i - s);
    b += 2;
    d = an * d + b;
    if (Math.abs(d) < 1e-30) d = 1e-30;
    c = b + an / c;
    if (Math.abs(c) < 1e-30) c = 1e-30;
    d = 1 / d;
    const del = d * c;
    h *= del;
    if (Math.abs(del - 1) < eps) break;
  }
  const gln = logGamma(s);
  return Math.exp(-x + s * Math.log(x) - gln) * h;
}

/** 两比例 Z 检验（如 A/B 两组转化率） */
export function twoProportionZTest(
  successA: number,
  nA: number,
  successB: number,
  nB: number,
  confidence = 0.95,
): SignificanceResult {
  if (nA === 0 || nB === 0) {
    return { statistic: 0, pValue: 1, isSignificant: false };
  }
  const pA = successA / nA;
  const pB = successB / nB;
  const pPool = (successA + successB) / (nA + nB);
  const se = Math.sqrt(pPool * (1 - pPool) * (1 / nA + 1 / nB));
  if (se === 0) return { statistic: 0, pValue: 1, isSignificant: false, effectSize: 0 };
  const z = (pA - pB) / se;
  const pValue = 2 * (1 - normalCDF(Math.abs(z)));
  return {
    statistic: z,
    pValue,
    isSignificant: pValue < 1 - confidence,
    effectSize: Math.abs(pA - pB),
  };
}

/** 便捷：根据 p 值判断是否显著 */
export function isSignificant(pValue: number, alpha = 0.05): boolean {
  return pValue < alpha;
}
