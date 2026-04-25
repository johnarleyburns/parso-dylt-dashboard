import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: string | null }

export default class MapErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(e: Error): State {
    return { error: e.message }
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#ff4444',
          fontFamily: 'monospace',
          fontSize: '0.65rem',
          letterSpacing: '0.08em',
          padding: '1rem',
          textAlign: 'center',
        }}>
          ⚠ MAP ERROR: {this.state.error}
        </div>
      )
    }
    return this.props.children
  }
}
