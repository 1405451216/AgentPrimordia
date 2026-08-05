/**
 * ErrorBoundary — 路由级错误边界
 *
 * 任一页面抛出未捕获错误时展示降级界面，避免白屏；
 * 记录错误详情并暴露「重试」出口。
 */
import { Component, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
  onReset?: () => void;
}

interface State {
  hasError: boolean;
  message: string;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, message: '' };

  static getDerivedStateFromError(error: unknown): State {
    return {
      hasError: true,
      message: error instanceof Error ? error.message : String(error),
    };
  }

  componentDidCatch(error: unknown) {
    // 记录错误详情，便于排查（生产可接入上报）
    console.error('[ErrorBoundary]', error);
  }

  handleReset = () => {
    this.setState({ hasError: false, message: '' });
    this.props.onReset?.();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="panel error">
          <h2>出错了</h2>
          <div className="error-panel" role="alert">
            <p className="error-msg">
              页面渲染时发生错误
              {this.state.message ? `：${this.state.message}` : '。'}
            </p>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={this.handleReset}>
                重试
              </button>
            </div>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
