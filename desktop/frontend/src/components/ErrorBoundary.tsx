import { Component, type ReactNode } from "react";
import { reportCrash } from "../lib/crash";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
}

interface ErrorBoundaryState {
  crashed: boolean;
  error: unknown;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { crashed: false, error: null };

  static getDerivedStateFromError(error: unknown) {
    return { crashed: true, error };
  }

  componentDidCatch(error: unknown, info: { componentStack?: string | null }) {
    reportCrash("react", error, info.componentStack ?? undefined);
  }

  render() {
    if (this.state.crashed) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div style={{ padding: "16px", color: "#e0696a", fontSize: "13px" }}>
          <p style={{ margin: "0 0 8px" }}>组件渲染出错</p>
          <button
            type="button"
            onClick={() => this.setState({ crashed: false, error: null })}
            style={{ padding: "4px 12px", border: "1px solid #343945", borderRadius: "6px", background: "#191b22", color: "#f4f5f7", cursor: "pointer" }}
          >
            重试
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
