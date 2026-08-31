/**
 * stats.ts — V7 弧线 S0-1：显著性检验与功效分析框架（TypeScript 端）。
 *
 * 与 Go 端 internal/eval/stats.go 逐函数同口径（铁律 5 双语言同步）：
 * - 比率类指标一律「点估计 + Wilson 95% 下界」（R3），不裸报 100%/0；
 * - 同题双臂配对用 McNemar 精确二项检验（成本最低的合法设计）；
 * - 样本量由精确枚举的功效函数算出（sampleSizeMcNemar），不查表拍数；
 * - 均值类差值（token/轮数成本）用配对 bootstrap 95% CI，不假设正态；
 * - judge 标定用 Cohen κ（S0-1 要求 κ ≥0.6）。
 *
 * 数值口径纪律：
 * - normalQuantile 与 Go 同算法（200 次二分，区间 [-12,12]，同迭代次数）；
 *   erfc 为 Go math/erfc.go（fdlibm s_erf.c 系）的逐算子移植，仅 Exp 一处
 *   换 Math.exp（两者均 ≤1 ulp 误差），分位数末位漂移 ≪1e-9 对账容差；
 * - xorshift64 用 BigInt 模拟 uint64 回绕，与 Go newLCG/next/intn 逐位一致
 *   （已验证 seed=20260831 前四个输出相同），bootstrap 跨语言可复现；
 * - 2^-n 用 Math.pow(0.5, n)（已验证与 Go math.Ldexp(1,-n) 的逐次减半语义
 *   在 n∈[0,1100] 全域位级一致）；
 * - 入参非法一律 throw Error，message 前缀与 Go ErrInvalidStatInput 一致
 *   （"eval: 统计入参非法"）。
 *
 * 跨语言数值对账门：agentprimordia/internal/eval/testdata/stats_fixtures.json
 * （Go 为权威生成方），由 src/eval/__tests__/stats.test.ts 逐项断言。
 */

// ===== 常量与错误 =====

/** 双侧 95% 置信的正态分位数（与 Go Z95 常量一致）。 */
export const Z95 = 1.959963984540054;

/** 与 Go ErrInvalidStatInput 对齐的报错前缀（调用方错误，不可重试）。 */
const ERR_PREFIX = 'eval: 统计入参非法';

/** 构造带统一前缀的入参非法错误。 */
function statInputError(detail: string): Error {
  return new Error(ERR_PREFIX + ': ' + detail);
}

/** 校验整数参数（Go 侧由 int 类型静态保证，TS 侧运行时对齐同一契约）。 */
function requireInt(name: string, value: number): void {
  if (!Number.isInteger(value)) {
    throw statInputError(name + '=' + value + ' 须为整数');
  }
}

/** 校验有限浮点参数（Go 侧 NaN 会静默通过比较类校验并产出垃圾值，TS 侧显式拒绝）。 */
function requireFinite(name: string, value: number): void {
  if (!Number.isFinite(value)) {
    throw statInputError(name + '=' + value + ' 须为有限数');
  }
}

// ===== erfc（Go math/erfc.go 的逐算子移植，供 normalCDF 使用） =====

// fdlibm/Go 同名系数：erf 在 [0, 0.84375] 的 P/Q 逼近
const erx = 8.45062911510467529297e-01; // 0x3FEB0AC160000000
const pp0 = 1.28379167095512558561e-01; // 0x3FC06EBA8214DB68
const pp1 = -3.25042107247001499370e-01; // 0xBFD4CD7D691CB913
const pp2 = -2.84817495755985104766e-02; // 0xBF9D2A51DBD7194F
const pp3 = -5.77027029648944159157e-03; // 0xBF77A291236668E4
const pp4 = -2.37630166566501626084e-05; // 0xBEF8EAD6120016AC
const qq1 = 3.97917223959155352819e-01; // 0x3FD97779CDDADC09
const qq2 = 6.50222499887672944485e-02; // 0x3FB0A54C5536CEBA
const qq3 = 5.08130628187576562776e-03; // 0x3F74D022C4D36B0F
const qq4 = 1.32494738004321644526e-04; // 0x3F215DC9221C1A10
const qq5 = -3.96022827877536812320e-06; // 0xBED09C4342A26120
// erf 在 [0.84375, 1.25] 的 P1/Q1 逼近
const pa0 = -2.36211856075265944077e-03; // 0xBF6359B8BEF77538
const pa1 = 4.14856118683748331666e-01; // 0x3FDA8D00AD92B34D
const pa2 = -3.72207876035701323847e-01; // 0xBFD7D240FBB8C3F1
const pa3 = 3.18346619901161753674e-01; // 0x3FD45FCA805120E4
const pa4 = -1.10894694282396677476e-01; // 0xBFBC63983D3E28EC
const pa5 = 3.54783043256182359371e-02; // 0x3FA22A36599795EB
const pa6 = -2.16637559486879084300e-03; // 0xBF61BF380A96073F
const qa1 = 1.06420880400844228286e-01; // 0x3FBB3E6618EEE323
const qa2 = 5.40397917702171048937e-01; // 0x3FE14AF092EB6F33
const qa3 = 7.18286544141962662868e-02; // 0x3FB2635CD99FE9A7
const qa4 = 1.26171219808761642112e-01; // 0x3FC02660E763351F
const qa5 = 1.36370839120290507362e-02; // 0x3F8BEDC26B51DD1C
const qa6 = 1.19844998467991074170e-02; // 0x3F888B545735151D
// erfc 在 [1.25, 1/0.35] 的 R1/S1 逼近
const ra0 = -9.86494403484714822705e-03; // 0xBF843412600D6435
const ra1 = -6.93858572707181764372e-01; // 0xBFE63416E4BA7360
const ra2 = -1.05586262253232909814e+01; // 0xC0251E0441B0E726
const ra3 = -6.23753324503260060396e+01; // 0xC04F300AE4CBA38D
const ra4 = -1.62396669462573470355e+02; // 0xC0644CB184282266
const ra5 = -1.84605092906711035994e+02; // 0xC067135CEBCCABB2
const ra6 = -8.12874355063065934246e+01; // 0xC054526557E4D2F2
const ra7 = -9.81432934416914548592e+00; // 0xC023A0EFC69AC25C
const sa1 = 1.96512716674392571292e+01; // 0x4033A6B9BD707687
const sa2 = 1.37657754143519042600e+02; // 0x4061350C526AE721
const sa3 = 4.34565877475229228821e+02; // 0x407B290DD58A1A71
const sa4 = 6.45387271733267880336e+02; // 0x40842B1921EC2868
const sa5 = 4.29008140027567833386e+02; // 0x407AD02157700314
const sa6 = 1.08635005541779435134e+02; // 0x405B28A3EE48AE2C
const sa7 = 6.57024977031928170135e+00; // 0x401A47EF8E484A93
const sa8 = -6.04244152148580987438e-02; // 0xBFAEEFF2EE749A62
// erfc 在 [1/0.35, 28] 的 R2/S2 逼近
const rb0 = -9.86494292470009928597e-03; // 0xBF84341239E86F4A
const rb1 = -7.99283237680523006574e-01; // 0xBFE993BA70C285DE
const rb2 = -1.77579549177547519889e+01; // 0xC031C209555F995A
const rb3 = -1.60636384855821916062e+02; // 0xC064145D43C5ED98
const rb4 = -6.37566443368389627722e+02; // 0xC083EC881375F228
const rb5 = -1.02509513161107724954e+03; // 0xC09004616A2E5992
const rb6 = -4.83519191608651397019e+02; // 0xC07E384E9BDC383F
const sb1 = 3.03380607434824582924e+01; // 0x403E568B261D5190
const sb2 = 3.25792512996573918826e+02; // 0x40745CAE221B9F0A
const sb3 = 1.53672958608443695994e+03; // 0x409802EB189D5118
const sb4 = 3.19985821950859553908e+03; // 0x40A8FFB7688C246A
const sb5 = 2.55305040643316442583e+03; // 0x40A3F219CEDF3BE6
const sb6 = 4.74528541206955367215e+02; // 0x407DA874E79FE763
const sb7 = -2.24409524465858183362e+01; // 0xC03670E242712D62

const F64 = new Float64Array(1);
const U32 = new Uint32Array(F64.buffer);
// 字节序探测：1.5 = 0x3FF8000000000000，小端时高 32 位落在 U32[1]
F64[0] = 1.5;
const LITTLE_ENDIAN = U32[0] === 0 && U32[1] === 0x3ff80000;

/** 伪单精度截断：清掉 double 位型的低 32 位（对齐 Go Float64frombits(Float64bits(x)&0xffffffff00000000)）。 */
function pseudoSingle(x: number): number {
  F64[0] = x;
  if (LITTLE_ENDIAN) {
    U32[0] = 0;
  } else {
    U32[1] = 0;
  }
  return F64[0];
}

/** erfc(x)：Go math/erfc.go 的逐算子移植（fdlibm s_erf.c 系），NaN/±Inf 特判与 Go 一致。 */
function erfc(x: number): number {
  const Tiny = 2 ** -56; // Go: 1.0 / (1 << 56)
  if (Number.isNaN(x)) return NaN;
  if (x === Infinity) return 0;
  if (x === -Infinity) return 2;
  let sign = false;
  if (x < 0) {
    x = -x;
    sign = true;
  }
  if (x < 0.84375) {
    // |x| < 0.84375
    let temp: number;
    if (x < Tiny) {
      temp = x;
    } else {
      const z = x * x;
      const r = pp0 + z * (pp1 + z * (pp2 + z * (pp3 + z * pp4)));
      const s = 1 + z * (qq1 + z * (qq2 + z * (qq3 + z * (qq4 + z * qq5))));
      const y = r / s;
      if (x < 0.25) {
        temp = x + x * y;
      } else {
        temp = 0.5 + (x * y + (x - 0.5));
      }
    }
    return sign ? 1 + temp : 1 - temp;
  }
  if (x < 1.25) {
    // 0.84375 <= |x| < 1.25
    const s = x - 1;
    const P = pa0 + s * (pa1 + s * (pa2 + s * (pa3 + s * (pa4 + s * (pa5 + s * pa6)))));
    const Q = 1 + s * (qa1 + s * (qa2 + s * (qa3 + s * (qa4 + s * (qa5 + s * qa6)))));
    return sign ? 1 + erx + P / Q : 1 - erx - P / Q;
  }
  if (x < 28) {
    // 1.25 <= |x| < 28（唯一引用 Exp 的分支；Go 用 math.Exp，TS 用 Math.exp，均 ≤1 ulp）
    const s = 1 / (x * x);
    let R: number;
    let S: number;
    if (x < 1 / 0.35) {
      R = ra0 + s * (ra1 + s * (ra2 + s * (ra3 + s * (ra4 + s * (ra5 + s * (ra6 + s * ra7))))));
      S = 1 + s * (sa1 + s * (sa2 + s * (sa3 + s * (sa4 + s * (sa5 + s * (sa6 + s * (sa7 + s * sa8)))))));
    } else {
      if (sign && x > 6) return 2; // x < -6
      R = rb0 + s * (rb1 + s * (rb2 + s * (rb3 + s * (rb4 + s * (rb5 + s * rb6)))));
      S = 1 + s * (sb1 + s * (sb2 + s * (sb3 + s * (sb4 + s * (sb5 + s * (sb6 + s * sb7))))));
    }
    const z = pseudoSingle(x);
    const r = Math.exp(-z * z - 0.5625) * Math.exp((z - x) * (z + x) + R / S);
    return sign ? 2 - r / x : r / x;
  }
  return sign ? 2 : 0;
}

// ===== 正态分布原语 =====

/** 标准正态累积分布 Φ(x)。与 Go NormalCDF 同式：0.5·erfc(−x/√2)。 */
export function normalCDF(x: number): number {
  return 0.5 * erfc(-x / Math.SQRT2);
}

/**
 * 标准正态分位数 Φ⁻¹(p)，0<p<1。
 * 与 Go NormalQuantile 同算法：区间 [-12,12]、200 次二分、同比较方向，
 * 保证与 Go 结果末位一致（差仅来自 erfc 的 ≤1 ulp 漂移，≪1e-9 对账容差）。
 */
export function normalQuantile(p: number): number {
  // 注：Go 侧 NaN/±Inf 会静默落入二分并返回垃圾值，TS 侧按入参非法显式拒绝。
  if (!Number.isFinite(p) || p <= 0 || p >= 1) {
    throw statInputError('p=' + p + ' 须落在 (0,1)');
  }
  let lo = -12;
  let hi = 12;
  for (let i = 0; i < 200; i++) {
    const mid = (lo + hi) / 2;
    if (normalCDF(mid) < p) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  return (lo + hi) / 2;
}

// ===== Wilson 区间（R3：比率类指标强制口径） =====

/** Wilson 区间结果：[lower, upper]。 */
export interface WilsonInterval {
  lower: number;
  upper: number;
}

/** 成功数比例的 Wilson score 区间；z 取分位数（95% 双侧用 Z95）。 */
export function wilsonInterval(successes: number, trials: number, z: number): WilsonInterval {
  requireInt('successes', successes);
  requireInt('trials', trials);
  requireFinite('z', z);
  if (trials <= 0) {
    throw statInputError('trials=' + trials + ' 须 >0');
  }
  if (successes < 0 || successes > trials) {
    throw statInputError('successes=' + successes + ' trials=' + trials);
  }
  if (z <= 0) {
    throw statInputError('z=' + z);
  }
  const n = trials;
  const p = successes / n;
  const z2 = z * z;
  const denom = 1 + z2 / n;
  const center = (p + z2 / (2 * n)) / denom;
  const margin = (z * Math.sqrt((p * (1 - p)) / n + z2 / (4 * n * n))) / denom;
  return { lower: center - margin, upper: center + margin };
}

/** RatePoint 的 snake_case JSON 形态（与 Go RatePoint 的 json tag 对齐）。 */
export interface RatePointJSON {
  successes: number;
  trials: number;
  point: number;
  wilson_lower95: number;
  wilson_upper95: number;
}

/** 比率指标的 R3 报告单元：点估计与 Wilson 下界成对出现。 */
export class RatePoint {
  constructor(
    /** 成功数。 */
    public successes: number,
    /** 总试验数。 */
    public trials: number,
    /** 点估计 successes/trials。 */
    public point: number,
    /** Wilson 95% 下界。 */
    public wilsonLower: number,
    /** Wilson 95% 上界。 */
    public wilsonUpper: number,
  ) {}

  /** snake_case JSON（与 Go 落盘报告互换；JSON.stringify 自动调用）。 */
  toJSON(): RatePointJSON {
    return {
      successes: this.successes,
      trials: this.trials,
      point: this.point,
      wilson_lower95: this.wilsonLower,
      wilson_upper95: this.wilsonUpper,
    };
  }

  /** 评审可读口径（对齐 Go RatePoint.String()）。 */
  toString(): string {
    return formatRate(this);
  }
}

/** 按 R3 口径报告一个比率（成功率/复用率/拦截率/准确率）。 */
export function reportRate(successes: number, trials: number): RatePoint {
  const { lower, upper } = wilsonInterval(successes, trials, Z95);
  return new RatePoint(successes, trials, successes / trials, lower, upper);
}

/** 评审可读口径，与 Go RatePoint.String() 同文案："0.900 (Wilson95 下界 0.826, n=100)"。 */
export function formatRate(r: RatePoint): string {
  return r.point.toFixed(3) + ' (Wilson95 下界 ' + r.wilsonLower.toFixed(3) + ', n=' + r.trials + ')';
}

// ===== McNemar 精确二项检验（同题双臂配对） =====

/**
 * 双侧精确 McNemar 检验 p 值。
 * b = 仅基线臂成功数，c = 仅处理臂成功数（一致对不参与检验）；
 * p = 2·P(X ≤ min(b,c))，X~Bin(b+c, 0.5)，上限截到 1。
 */
export function mcnemarExact(b: number, c: number): number {
  requireInt('b', b);
  requireInt('c', c);
  if (b < 0 || c < 0) {
    throw statInputError('b=' + b + ' c=' + c + ' 不可为负');
  }
  const n = b + c;
  if (n === 0) {
    return 1; // 无不一致对：无法拒绝原假设
  }
  const m = Math.min(b, c);
  // Go: term = math.Ldexp(1, -n)（精确 2 的幂避免下溢）。
  // Math.pow(0.5, n) 已验证与逐次减半（即 Ldexp 语义）在 n∈[0,1100] 位级一致。
  let term = Math.pow(0.5, n);
  let sum = term;
  for (let k = 1; k <= m; k++) {
    term *= (n - k + 1) / k;
    sum += term;
  }
  let p = 2 * sum;
  if (p > 1) {
    p = 1;
  }
  return p;
}

/** 同一题面在双臂上的一次配对判定。 */
export interface PairedOutcome {
  /** 题目标识。 */
  taskId: string;
  /** 基线臂是否成功。 */
  baseline: boolean;
  /** 处理臂是否成功。 */
  treatment: boolean;
}

/** PairedAnalysis 的 snake_case JSON 形态（与 Go PairedAnalysis 的 json tag 对齐）。 */
export interface PairedAnalysisJSON {
  n: number;
  concordant: number;
  disc_b: number;
  disc_c: number;
  lift: number;
  p_value: number;
  baseline_rate: RatePointJSON;
  treatment_rate: RatePointJSON;
}

/** McNemar 配对分析结果（含双臂 Wilson 口径）。 */
export class PairedAnalysis {
  constructor(
    /** 配对题数。 */
    public n: number,
    /** 两臂结果一致的题数。 */
    public concordant: number,
    /** 仅基线臂成功数（b）。 */
    public discB: number,
    /** 仅处理臂成功数（c）。 */
    public discC: number,
    /** 处理臂 − 基线臂成功率差。 */
    public lift: number,
    /** McNemar 精确 p 值。 */
    public pValue: number,
    /** 基线臂 R3 口径。 */
    public baselineRate: RatePoint,
    /** 处理臂 R3 口径。 */
    public treatmentRate: RatePoint,
  ) {}

  /** snake_case JSON（与 Go 落盘报告互换；JSON.stringify 自动调用）。 */
  toJSON(): PairedAnalysisJSON {
    return {
      n: this.n,
      concordant: this.concordant,
      disc_b: this.discB,
      disc_c: this.discC,
      lift: this.lift,
      p_value: this.pValue,
      baseline_rate: this.baselineRate.toJSON(),
      treatment_rate: this.treatmentRate.toJSON(),
    };
  }
}

/** 同题双臂结果的 McNemar 配对分析。 */
export function analyzePaired(pairs: readonly PairedOutcome[]): PairedAnalysis {
  if (pairs.length === 0) {
    throw statInputError('配对结果为空');
  }
  let b = 0;
  let c = 0;
  let baseOK = 0;
  let treatOK = 0;
  for (const p of pairs) {
    if (p.baseline) baseOK++;
    if (p.treatment) treatOK++;
    if (p.baseline && !p.treatment) b++;
    if (!p.baseline && p.treatment) c++;
  }
  const pValue = mcnemarExact(b, c);
  const baselineRate = reportRate(baseOK, pairs.length);
  const treatmentRate = reportRate(treatOK, pairs.length);
  return new PairedAnalysis(
    pairs.length,
    pairs.length - b - c,
    b,
    c,
    (treatOK - baseOK) / pairs.length,
    pValue,
    baselineRate,
    treatmentRate,
  );
}

// ===== 两独立比例检验与样本量（无法配对时的退路） =====

/** 两独立比例 z 检验结果：差值（p2−p1）、z 统计量、双侧 p。 */
export interface TwoProportionZResult {
  diff: number;
  z: number;
  pValue: number;
}

/** 两独立比例 z 检验（合并方差）：x/n 为各臂成功数与总数。 */
export function twoProportionZTest(x1: number, n1: number, x2: number, n2: number): TwoProportionZResult {
  requireInt('x1', x1);
  requireInt('n1', n1);
  requireInt('x2', x2);
  requireInt('n2', n2);
  if (n1 <= 0 || n2 <= 0 || x1 < 0 || x2 < 0 || x1 > n1 || x2 > n2) {
    throw statInputError('(' + x1 + '/' + n1 + ') vs (' + x2 + '/' + n2 + ')');
  }
  const p1 = x1 / n1;
  const p2 = x2 / n2;
  const pool = (x1 + x2) / (n1 + n2);
  const se = Math.sqrt(pool * (1 - pool) * (1 / n1 + 1 / n2));
  if (se === 0) {
    return { diff: p2 - p1, z: 0, pValue: 1 };
  }
  const z = (p2 - p1) / se;
  return { diff: p2 - p1, z, pValue: 2 * (1 - normalCDF(Math.abs(z))) };
}

/** 独立双臂每组样本量（正态近似，双侧 alpha）。 */
export function sampleSizeTwoProportion(p1: number, p2: number, alpha: number, targetPower: number): number {
  requireFinite('p1', p1);
  requireFinite('p2', p2);
  requireFinite('alpha', alpha);
  requireFinite('targetPower', targetPower);
  if (p1 <= 0 || p1 >= 1 || p2 <= 0 || p2 >= 1 || p1 === p2) {
    throw statInputError('p1=' + p1 + ' p2=' + p2);
  }
  if (alpha <= 0 || alpha >= 1 || targetPower <= 0 || targetPower >= 1) {
    throw statInputError('alpha=' + alpha + ' power=' + targetPower);
  }
  const zA = normalQuantile(1 - alpha / 2);
  const zB = normalQuantile(targetPower);
  const pool = (p1 + p2) / 2;
  const num = zA * Math.sqrt(2 * pool * (1 - pool)) + zB * Math.sqrt(p1 * (1 - p1) + p2 * (1 - p2));
  return Math.ceil((num * num) / ((p2 - p1) * (p2 - p1)));
}

// ===== McNemar 功效精确枚举与样本量（R2 的落地函数） =====

/**
 * 精确枚举配对设计功效。
 * n=题数；delta=真实成功率差（处理−基线，如 0.15）；omega=真实不一致率（须 >|delta|）。
 * 模型：b~Bin(n,(ω+δ)/2)、c~Bin(n,(ω−δ)/2) 独立；McNemarExact(b,c) ≤ alpha 判为拒绝。
 */
export function mcnemarPower(n: number, delta: number, omega: number, alpha: number): number {
  requireInt('n', n);
  requireFinite('delta', delta);
  requireFinite('omega', omega);
  requireFinite('alpha', alpha);
  if (n <= 0) {
    throw statInputError('n=' + n);
  }
  if (alpha <= 0 || alpha >= 1) {
    throw statInputError('alpha=' + alpha);
  }
  if (omega <= 0 || omega > 1) {
    throw statInputError('omega=' + omega);
  }
  if (Math.abs(delta) >= omega) {
    throw statInputError('|delta|=' + Math.abs(delta) + ' 须 < omega=' + omega + '（不一致率必须容得下真实差值）');
  }
  const p10 = (omega + delta) / 2; // 仅处理臂成功
  const p01 = (omega - delta) / 2; // 仅基线臂成功
  const rej = rejectionTable(n, alpha);
  const pb = binomialPMF(n, p10);
  const pc = binomialPMF(n, p01);
  let power = 0;
  for (let i = 0; i <= n; i++) {
    if (pb[i] <= 0) {
      continue;
    }
    for (let j = 0; j <= n; j++) {
      if (pc[j] <= 0) {
        continue;
      }
      if (rej[i][j]) {
        power += pb[i] * pc[j];
      }
    }
  }
  return power;
}

/**
 * 达到 targetPower 所需的最小题数（正态近似定起点 + 精确枚举二分 + 向下逐步校正）。
 * R2「样本量由设计算出而非拍 30」的落地入口；三步搜索与 Go SampleSizeMcNemar 逐句同构。
 */
export function sampleSizeMcNemar(delta: number, omega: number, alpha: number, targetPower: number): number {
  requireFinite('delta', delta);
  requireFinite('omega', omega);
  requireFinite('alpha', alpha);
  requireFinite('targetPower', targetPower);
  if (Math.abs(delta) <= 0 || Math.abs(delta) >= omega) {
    throw statInputError('delta=' + delta + ' omega=' + omega);
  }
  if (targetPower <= 0 || targetPower >= 1) {
    throw statInputError('targetPower=' + targetPower);
  }
  if (alpha <= 0 || alpha >= 1) {
    throw statInputError('alpha=' + alpha);
  }
  const zA = normalQuantile(1 - alpha / 2);
  const zB = normalQuantile(targetPower);
  // 正态近似只作搜索起点（可能偏大也可能偏小）：先把它当区间上界的种子，
  // 再用精确枚举二分找最小达标 n——不能只向上走，否则近似偏大时会虚报样本量。
  const maxN = 4000;
  const meets = (n: number): boolean => mcnemarPower(n, delta, omega, alpha) >= targetPower;
  let hi = Math.ceil(((zA + zB) * (zA + zB) * omega) / (delta * delta));
  if (hi < 1) {
    hi = 1;
  }
  for (;;) {
    if (hi >= maxN) {
      throw statInputError('maxN=' + maxN + ' 内达不到 power=' + targetPower + '（delta=' + delta + ' omega=' + omega + '）');
    }
    if (meets(hi)) {
      break;
    }
    hi *= 2;
  }
  let lo = 1;
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2); // Go: (lo+hi)/2 为 int 整除，JS 需显式取整
    if (meets(mid)) {
      hi = mid;
    } else {
      lo = mid + 1;
    }
  }
  // 离散非严格单调：向下逐步校正到真正的局部最小达标点
  while (lo > 1) {
    if (!meets(lo - 1)) {
      break;
    }
    lo--;
  }
  return lo;
}

/** 预计算 (b,c) 网格上的「是否拒绝 H0」，尾概率按 t=b+c 递推复用（Go rejectionTable 同构）。 */
function rejectionTable(n: number, alpha: number): boolean[][] {
  const tails: number[][] = [];
  for (let t = 0; t <= 2 * n; t++) {
    const row: number[] = new Array(t + 1);
    let term = Math.pow(0.5, t); // Go: math.Ldexp(1, -t)
    let sum = term;
    row[0] = sum;
    for (let k = 1; k <= t; k++) {
      term *= (t - k + 1) / k;
      sum += term;
      row[k] = sum;
    }
    tails.push(row);
  }
  const rej: boolean[][] = [];
  for (let b = 0; b <= n; b++) {
    rej.push(new Array<boolean>(n + 1).fill(false));
  }
  for (let b = 0; b <= n; b++) {
    for (let c = 0; c <= n; c++) {
      const t = b + c;
      if (t === 0) {
        continue;
      }
      const m = Math.min(b, c);
      let p = 2 * tails[t][m];
      if (p > 1) {
        p = 1;
      }
      rej[b][c] = p <= alpha;
    }
  }
  return rej;
}

/** 返回 k=0..n 的二项概率质量（递推，容忍极端 p；Go binomialPMF 同构）。 */
function binomialPMF(n: number, p: number): number[] {
  const out: number[] = new Array(n + 1).fill(0);
  if (p <= 0) {
    out[0] = 1;
    return out;
  }
  if (p >= 1) {
    out[n] = 1;
    return out;
  }
  let pmf = Math.pow(1 - p, n);
  out[0] = pmf;
  for (let k = 1; k <= n; k++) {
    pmf *= ((n - k + 1) / k) * (p / (1 - p));
    out[k] = pmf;
  }
  return out;
}

// ===== Cohen κ（judge 标定，S0-1 要求 κ ≥0.6） =====

/** 两名评分者的未加权一致性系数。输入等长标签数组。 */
export function cohenKappa(raterA: readonly string[], raterB: readonly string[]): number {
  if (raterA.length === 0 || raterA.length !== raterB.length) {
    throw statInputError('标签切片为空或长度不等');
  }
  const cats = new Set<string>();
  for (const s of raterA) cats.add(s);
  for (const s of raterB) cats.add(s);
  const n = raterA.length;
  let agree = 0;
  const cntA = new Map<string, number>();
  const cntB = new Map<string, number>();
  for (let i = 0; i < n; i++) {
    if (raterA[i] === raterB[i]) agree++;
    cntA.set(raterA[i], (cntA.get(raterA[i]) ?? 0) + 1);
    cntB.set(raterB[i], (cntB.get(raterB[i]) ?? 0) + 1);
  }
  const po = agree / n;
  let pe = 0;
  for (const c of cats) {
    // 类别遍历顺序不影响对账（Go 侧 map 随机序，求和差异 ≤1 ulp ≪1e-9 容差）
    pe += ((cntA.get(c) ?? 0) / n) * ((cntB.get(c) ?? 0) / n);
  }
  if (pe === 1) {
    throw statInputError('期望一致率为 1（单一类别），κ 无定义');
  }
  return (po - pe) / (1 - pe);
}

// ===== 配对 bootstrap CI（成本类差值，不假设正态） =====

/** BootstrapCI 的 snake_case JSON 形态（与 Go BootstrapCI 的 json tag 对齐）。 */
export interface BootstrapCIJSON {
  point: number;
  lower95: number;
  upper95: number;
  iterations: number;
  seed: number;
}

/** 自助法置信区间。 */
export class BootstrapCI {
  constructor(
    /** 全样本均值差（点估计）。 */
    public point: number,
    /** bootstrap 2.5% 分位（下界）。 */
    public lower: number,
    /** bootstrap 97.5% 分位（上界）。 */
    public upper: number,
    /** 重采样次数。 */
    public iterations: number,
    /** 随机种子（固定保证可复现）。 */
    public seed: number,
  ) {}

  /** snake_case JSON（与 Go 落盘报告互换；JSON.stringify 自动调用）。 */
  toJSON(): BootstrapCIJSON {
    return {
      point: this.point,
      lower95: this.lower,
      upper95: this.upper,
      iterations: this.iterations,
      seed: this.seed,
    };
  }
}

const MASK64 = 0xffffffffffffffffn;

/**
 * 确定性 xorshift64 伪随机源（BigInt 模拟 uint64 回绕）。
 * 与 Go newLCG/state/next/intn 逐位一致：state = uint64(seed)|1；
 * next(): s ^= s<<13; s ^= s>>>7; s ^= s<<17（64 位回绕）。私有实现，
 * 跨语言可复现性由夹具门（bootstrap 条目逐位一致）保证。
 */
class Xorshift64 {
  private state: bigint;

  constructor(seed: number) {
    // Go: uint64(seed) | 1 —— 负数 seed 按 64 位补码位型取值
    this.state = BigInt.asUintN(64, BigInt(seed)) | 1n;
  }

  /** 推进状态并返回 64 位输出。 */
  next(): bigint {
    let s = this.state;
    s ^= (s << 13n) & MASK64; // uint64 左移回绕
    s ^= s >> 7n; // state 恒非负，BigInt >> 即无符号右移
    s ^= (s << 17n) & MASK64;
    this.state = s;
    return s;
  }

  /** Go lcg.intn：next() % n。 */
  intn(n: number): number {
    return Number(this.next() % BigInt(n));
  }
}

/**
 * 对每题差值序列做均值 percentile bootstrap。
 * deltas[i] = 处理臂观测 − 基线臂观测（同题）；iterations ≥100；seed 固定保证可复现。
 * 注：seed 为 Go int64 位型，TS number 仅能精确表达 ±2^53 内的种子（夹具种子均满足）。
 */
export function pairedBootstrapCI(deltas: readonly number[], iterations: number, seed: number): BootstrapCI {
  if (deltas.length === 0) {
    throw statInputError('差值序列为空');
  }
  requireInt('iterations', iterations);
  if (iterations < 100) {
    throw statInputError('iterations=' + iterations + ' 须 ≥100');
  }
  let point = 0;
  for (const d of deltas) {
    if (!Number.isFinite(d)) {
      // Go: math.IsNaN(d) || math.IsInf(d, 0)
      throw statInputError('差值序列含非有限值');
    }
    point += d;
  }
  point /= deltas.length;
  const rng = new Xorshift64(seed);
  const means: number[] = new Array(iterations);
  const n = deltas.length;
  for (let it = 0; it < iterations; it++) {
    let sum = 0;
    for (let i = 0; i < n; i++) {
      sum += deltas[rng.intn(n)];
    }
    means[it] = sum / n;
  }
  // 升序排序；值全等元素顺序不影响分位取值
  means.sort((a, b) => a - b);
  const lo = means[Math.trunc(0.025 * iterations)];
  const hi = means[Math.trunc(0.975 * iterations) - 1];
  return new BootstrapCI(point, lo, hi, iterations, seed);
}
