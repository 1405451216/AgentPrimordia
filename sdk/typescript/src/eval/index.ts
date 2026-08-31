/**
 * eval 模块公共出口（barrel）。
 *
 * 当前仅导出 V7 弧线 S0-1 统计框架（对齐 Go internal/eval/stats.go，
 * 双线数值对账门见 internal/eval/testdata/stats_fixtures.json）；
 * shared-cases / benchmark-cases 等既有模块暂不纳入，避免扩大公共 API 面。
 */
export {
  Z95,
  normalCDF,
  normalQuantile,
  wilsonInterval,
  reportRate,
  formatRate,
  mcnemarExact,
  analyzePaired,
  twoProportionZTest,
  sampleSizeTwoProportion,
  mcnemarPower,
  sampleSizeMcNemar,
  cohenKappa,
  pairedBootstrapCI,
  RatePoint,
  PairedAnalysis,
  BootstrapCI,
} from './stats.js';
export type {
  WilsonInterval,
  RatePointJSON,
  PairedOutcome,
  PairedAnalysisJSON,
  TwoProportionZResult,
  BootstrapCIJSON,
} from './stats.js';
