/**
 * 站内帮助页
 *
 * 替代裸 markdown 帮助文档：样式化呈现面板说明、快捷键、
 * 混沌实验语义与数据来源，补全市场/学习章节。
 */
import { PageTitle } from '../PageTitle';

export function HelpPage() {
  return (
    <div className="panel help-page">
      <PageTitle title="帮助文档" subtitle="Help" />

      <section>
        <h3>面板</h3>
        <table>
          <thead>
            <tr><th>路由</th><th>面板</th><th>说明</th></tr>
          </thead>
          <tbody>
            <tr><td><code>/</code></td><td>概览</td><td>集群健康、知识蒸馏、近期实验一览</td></tr>
            <tr><td><code>/chaos</code></td><td>混沌实验</td><td>创建/运行混沌实验（故障注入）、查看实验报告</td></tr>
            <tr><td><code>/cluster</code></td><td>集群</td><td>节点拓扑、领导者状态、分片分布</td></tr>
            <tr><td><code>/learning</code></td><td>学习监控</td><td>知识蒸馏统计、能力进化趋势</td></tr>
            <tr><td><code>/marketplace</code></td><td>Agent 市场</td><td>模板浏览/搜索/部署，管理已部署 Agent</td></tr>
          </tbody>
        </table>
      </section>

      <section>
        <h3>键盘快捷键</h3>
        <table>
          <thead>
            <tr><th>按键</th><th>作用</th></tr>
          </thead>
          <tbody>
            <tr><td><kbd>Shift + /</kbd></td><td>打开/关闭快捷键面板</td></tr>
            <tr><td><kbd>/</kbd></td><td>聚焦当前页搜索框</td></tr>
            <tr><td><kbd>g 1-5</kbd></td><td>跳转页面：1 概览 · 2 混沌 · 3 集群 · 4 学习 · 5 市场</td></tr>
            <tr><td><kbd>Esc</kbd></td><td>关闭对话框或面板</td></tr>
          </tbody>
        </table>
      </section>

      <section>
        <h3>混沌实验</h3>
        <p>混沌实验用于验证系统在故障注入下的稳态表现：</p>
        <ul>
          <li>实验前系统应处于稳态基线（稳态前）</li>
          <li>注入故障（延迟 / 错误 / 超时 / 网络分区 / 进程终止）</li>
          <li>实验后检查系统是否回到稳态（稳态后）</li>
          <li>假设验证 = 实验是否证明了你的预期</li>
        </ul>
        <p className="help-note">
          破坏性故障类型（网络分区、进程终止）会经过两步确认，请确认影响范围后再运行。
        </p>
      </section>

      <section>
        <h3>Agent 市场</h3>
        <p>
          浏览模板并一键部署到当前集群，创建对应的 Agent 实例。
          部署后可在「已部署 Agent」面板中查看运行状态、停止或重新启动实例。
        </p>
      </section>

      <section>
        <h3>数据说明</h3>
        <p>
          界面数据来自 Studio 后端 <code>/api/v1/*</code> 端点。默认内置演示数据；
          接入真实引擎请参考 <code>internal/studio</code> 的
          <code> WithChaos / WithCluster / WithLearning / WithMarketplace</code> 选项。
        </p>
      </section>
    </div>
  );
}
