/**
 * ErrorBoundary — 根级错误边界
 *
 * 任一页面抛出未捕获错误时展示降级界面，避免白屏；
 * 提供「重试」回到正常渲染。
 */
import { Component, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="panel error">
          <h2>出错了</h2>
          <div className="error-panel" role="alert">
            <p className="error-msg">页面渲染时发生未预期错误。</p>
            <button
              className="btn-secondary"
              onClick={() => this.setState({ hasError: false })}
            >
              重试
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
