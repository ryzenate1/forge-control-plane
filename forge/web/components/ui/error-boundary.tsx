"use client";

import { Component, type ErrorInfo, type ReactNode } from "react";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
  /** Remount the subtree when this changes — resets error state. */
  resetKey?: string | number;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("[ErrorBoundary]", error, errorInfo);
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps) {
    if (prevProps.resetKey !== this.props.resetKey && this.state.hasError) {
      this.setState({ hasError: false, error: null });
    }
    if (prevProps.children !== this.props.children && this.state.hasError) {
      // A new subtree is not the same failure — allow a retry via remount.
      this.setState({ hasError: false, error: null });
    }
  }

  private retry = () => this.setState({ hasError: false, error: null });

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div className="rounded-xl border border-danger-line bg-danger-subtle p-6 text-center" role="alert">
          <h3 className="text-lg font-semibold text-danger">Something went wrong</h3>
          <p className="mt-2 text-sm text-text-subtle">{this.state.error?.message ?? "An unexpected error occurred."}</p>
          <button
            className="ui-button ui-button-danger mt-4"
            onClick={this.retry}
            type="button"
          >
            Try Again
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
