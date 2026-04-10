/**
 * Sigma Layout Controller
 *
 * Wraps graphology-layout-forceatlas2 web worker for async layout computation.
 * Manages start/stop lifecycle and auto-convergence.
 */

import type Graph from "graphology";
import FA2Layout from "graphology-layout-forceatlas2/worker";

const DEFAULT_SETTINGS = {
  gravity: 1,
  scalingRatio: 2,
  slowDown: 5,
  barnesHutOptimize: true,
  barnesHutTheta: 0.5,
  adjustSizes: true,
};

const AUTO_STOP_MS = 3_000;

export class LayoutController {
  private layout: FA2Layout | null = null;
  private stopTimer: ReturnType<typeof setTimeout> | null = null;
  private _isRunning = false;

  get isRunning(): boolean {
    return this._isRunning;
  }

  start(graph: Graph): void {
    if (typeof window === "undefined") return;
    this.stop();

    if (graph.order === 0) return;

    this.layout = new FA2Layout(graph, {
      settings: DEFAULT_SETTINGS,
    });
    this.layout.start();
    this._isRunning = true;

    this.stopTimer = setTimeout(() => {
      this.stop();
    }, AUTO_STOP_MS);
  }

  stop(): void {
    if (this.stopTimer) {
      clearTimeout(this.stopTimer);
      this.stopTimer = null;
    }
    if (this.layout) {
      this.layout.stop();
      this.layout.kill();
      this.layout = null;
    }
    this._isRunning = false;
  }
}
