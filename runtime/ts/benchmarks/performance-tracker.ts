/**
 * XPB Performance Tracker
 * 
 * Tracks performance over time and generates trend reports
 * Stores results locally in JSON format
 */

import * as fs from 'fs';
import * as path from 'path';

const HISTORY_DIR = path.join(__dirname, 'history');
const LATEST_FILE = path.join(HISTORY_DIR, 'latest.json');
const TRENDS_FILE = path.join(HISTORY_DIR, 'trends.json');

interface PerformanceSnapshot {
  timestamp: string;
  commit?: string;
  branch?: string;
  results: BenchmarkResult[];
  environment: EnvironmentInfo;
}

interface BenchmarkResult {
  name: string;
  feature: string;
  dataSize: number;
  opsPerSecond: number;
  avgTime: number;
  minTime: number;
  maxTime: number;
  stdDev: number;
}

interface EnvironmentInfo {
  browser?: string;
  browserVersion?: string;
  node?: string;
  platform: string;
  arch: string;
  cpus: number;
  memory: string;
}

interface TrendData {
  metric: string;
  dataPoints: {
    date: string;
    value: number;
    commit?: string;
  }[];
  trend: 'improving' | 'stable' | 'regressing';
  changePercent: number;
}

export class PerformanceTracker {
  private historyDir: string;

  constructor(customDir?: string) {
    this.historyDir = customDir || HISTORY_DIR;
    this.ensureDirectory();
  }

  private ensureDirectory(): void {
    if (!fs.existsSync(this.historyDir)) {
      fs.mkdirSync(this.historyDir, { recursive: true });
    }
  }

  /**
   * Save a performance snapshot
   */
  saveSnapshot(results: BenchmarkResult[], metadata?: { commit?: string; branch?: string }): void {
    const snapshot: PerformanceSnapshot = {
      timestamp: new Date().toISOString(),
      commit: metadata?.commit,
      branch: metadata?.branch,
      results,
      environment: this.detectEnvironment(),
    };

    // Save as latest
    fs.writeFileSync(LATEST_FILE, JSON.stringify(snapshot, null, 2));

    // Save to history with timestamp
    const date = new Date();
    const filename = `benchmark-${date.toISOString().split('T')[0]}-${Date.now()}.json`;
    const filepath = path.join(this.historyDir, filename);
    fs.writeFileSync(filepath, JSON.stringify(snapshot, null, 2));

    // Update trends
    this.updateTrends(snapshot);

    console.log(`✓ Performance snapshot saved: ${filepath}`);
  }

  /**
   * Load the latest snapshot
   */
  loadLatest(): PerformanceSnapshot | null {
    if (!fs.existsSync(LATEST_FILE)) {
      return null;
    }
    return JSON.parse(fs.readFileSync(LATEST_FILE, 'utf-8'));
  }

  /**
   * Load all historical snapshots
   */
  loadHistory(): PerformanceSnapshot[] {
    if (!fs.existsSync(this.historyDir)) {
      return [];
    }

    const files = fs
      .readdirSync(this.historyDir)
      .filter((f) => f.startsWith('benchmark-') && f.endsWith('.json'))
      .sort()
      .reverse();

    return files.map((f) =>
      JSON.parse(fs.readFileSync(path.join(this.historyDir, f), 'utf-8'))
    );
  }

  /**
   * Compare current results against baseline
   */
  compareAgainstBaseline(current: BenchmarkResult[]): ComparisonResult {
    const baseline = this.loadLatest();
    if (!baseline) {
      return { hasBaseline: false, comparisons: [] };
    }

    const comparisons: MetricComparison[] = [];

    for (const currentResult of current) {
      const baselineResult = baseline.results.find(
        (r) =>
          r.feature === currentResult.feature &&
          r.dataSize === currentResult.dataSize
      );

      if (baselineResult) {
        const opsChange =
          ((currentResult.opsPerSecond - baselineResult.opsPerSecond) /
            baselineResult.opsPerSecond) *
          100;

        comparisons.push({
          name: currentResult.name,
          feature: currentResult.feature,
          dataSize: currentResult.dataSize,
          baselineOps: baselineResult.opsPerSecond,
          currentOps: currentResult.opsPerSecond,
          changePercent: opsChange,
          status: this.getStatus(opsChange),
        });
      }
    }

    return { hasBaseline: true, comparisons };
  }

  /**
   * Generate trend analysis
   */
  analyzeTrends(metric: string, days = 30): TrendData | null {
    const history = this.loadHistory().slice(0, days);
    if (history.length < 2) {
      return null;
    }

    const dataPoints = history.map((snapshot) => {
      const result = snapshot.results.find((r) => r.name.includes(metric)
      );
      return {
        date: snapshot.timestamp,
        value: result?.opsPerSecond || 0,
        commit: snapshot.commit,
      };
    });

    // Calculate trend
    const first = dataPoints[0].value;
    const last = dataPoints[dataPoints.length - 1].value;
    const changePercent = ((last - first) / first) * 100;

    let trend: 'improving' | 'stable' | 'regressing' = 'stable';
    if (changePercent > 5) trend = 'improving';
    else if (changePercent < -5) trend = 'regressing';

    return {
      metric,
      dataPoints,
      trend,
      changePercent,
    };
  }

  /**
   * Generate performance report
   */
  generateReport(current?: BenchmarkResult[]): string {
    let report = '# XPB Performance Report\n\n';
    report += `Generated: ${new Date().toISOString()}\n\n`;

    // Environment info
    const env = this.detectEnvironment();
    report += '## Environment\n\n';
    report += `- Platform: ${env.platform} (${env.arch})\n`;
    report += `- CPUs: ${env.cpus}\n`;
    report += `- Memory: ${env.memory}\n`;
    if (env.browser) {
      report += `- Browser: ${env.browser} ${env.browserVersion}\n`;
    }
    if (env.node) {
      report += `- Node.js: ${env.node}\n`;
    }
    report += '\n';

    // Current results
    if (current) {
      report += '## Current Results\n\n';
      report += this.formatResultsTable(current);
    }

    // Comparison with baseline
    if (current) {
      const comparison = this.compareAgainstBaseline(current);
      if (comparison.hasBaseline) {
        report += '\n## Comparison with Baseline\n\n';
        report += this.formatComparisonTable(comparison.comparisons);

        const regressions = comparison.comparisons.filter(
          (c) => c.status === 'regression'
        );
        if (regressions.length > 0) {
          report += '\n### ⚠️ Regressions Detected\n\n';
          for (const reg of regressions) {
            report += `- **${reg.name}**: ${reg.changePercent.toFixed(1)}% slower\n`;
          }
        }

        const improvements = comparison.comparisons.filter(
          (c) => c.status === 'improvement'
        );
        if (improvements.length > 0) {
          report += '\n### 🚀 Improvements\n\n';
          for (const imp of improvements) {
            report += `- **${imp.name}**: +${imp.changePercent.toFixed(1)}% faster\n`;
          }
        }
      }
    }

    // Trends
    const trends = this.analyzeTrends('Baseline', 30);
    if (trends) {
      report += '\n## Performance Trends (30 days)\n\n';
      report += `**${trends.metric}**: ${trends.trend} (${trends.changePercent.toFixed(1)}%)\n\n`;
      report += '```\n';
      for (const point of trends.dataPoints.slice(0, 10)) {
        const date = new Date(point.date).toLocaleDateString();
        report += `${date}: ${point.value.toFixed(0)} ops/sec\n`;
      }
      report += '```\n';
    }

    // Historical context
    const history = this.loadHistory();
    if (history.length > 0) {
      report += '\n## Historical Context\n\n';
      report += `- Total snapshots: ${history.length}\n`;
      report += `- First recorded: ${history[history.length - 1].timestamp}\n`;
      report += `- Last recorded: ${history[0].timestamp}\n`;
    }

    return report;
  }

  private updateTrends(snapshot: PerformanceSnapshot): void {
    let trends: { [key: string]: TrendData } = {};

    if (fs.existsSync(TRENDS_FILE)) {
      trends = JSON.parse(fs.readFileSync(TRENDS_FILE, 'utf-8'));
    }

    for (const result of snapshot.results) {
      const key = `${result.feature}-${result.dataSize}`;
      if (!trends[key]) {
        trends[key] = {
          metric: result.name,
          dataPoints: [],
          trend: 'stable',
          changePercent: 0,
        };
      }

      trends[key].dataPoints.push({
        date: snapshot.timestamp,
        value: result.opsPerSecond,
        commit: snapshot.commit,
      });

      // Keep only last 90 days
      const cutoff = new Date();
      cutoff.setDate(cutoff.getDate() - 90);
      trends[key].dataPoints = trends[key].dataPoints.filter(
        (p) => new Date(p.date) > cutoff
      );
    }

    fs.writeFileSync(TRENDS_FILE, JSON.stringify(trends, null, 2));
  }

  private detectEnvironment(): EnvironmentInfo {
    const isNode = typeof process !== 'undefined';

    if (isNode) {
      return {
        node: process.version,
        platform: process.platform,
        arch: process.arch,
        cpus: require('os').cpus().length,
        memory:
          Math.round(require('os').totalmem() / (1024 * 1024 * 1024)) + 'GB',
      };
    }

    // Browser environment
    const nav = navigator as any;
    return {
      browser: nav.userAgent.match(/Chrome\/[\d.]+/)?.[0] || 'Unknown',
      browserVersion: nav.userAgent.match(/Chrome\/([\d.]+)/)?.[1],
      platform: nav.platform,
      arch: 'unknown',
      cpus: nav.hardwareConcurrency || 1,
      memory: nav.deviceMemory
        ? nav.deviceMemory + 'GB'
        : 'unknown',
    };
  }

  private getStatus(changePercent: number): 'improvement' | 'stable' | 'regression' {
    if (changePercent > 5) return 'improvement';
    if (changePercent < -10) return 'regression';
    return 'stable';
  }

  private formatResultsTable(results: BenchmarkResult[]): string {
    let table = '| Test | Data Size | Ops/sec | Time (ms) |\n';
    table += '|------|-----------|---------|-----------|\n';

    for (const r of results) {
      table += `| ${r.name} | ${r.dataSize}B | ${r.opsPerSecond.toFixed(0)} | ${r.avgTime.toFixed(3)} |\n`;
    }

    return table;
  }

  private formatComparisonTable(comparisons: MetricComparison[]): string {
    let table = '| Test | Baseline | Current | Change | Status |\n';
    table += '|------|----------|---------|--------|--------|\n';

    for (const c of comparisons) {
      const changeStr = c.changePercent > 0 ? `+${c.changePercent.toFixed(1)}%` : `${c.changePercent.toFixed(1)}%`;
      const statusIcon =
        c.status === 'improvement' ? '🟢' : c.status === 'regression' ? '🔴' : '🟡';
      table += `| ${c.name} | ${c.baselineOps.toFixed(0)} | ${c.currentOps.toFixed(0)} | ${changeStr} | ${statusIcon} |\n`;
    }

    return table;
  }
}

interface ComparisonResult {
  hasBaseline: boolean;
  comparisons: MetricComparison[];
}

interface MetricComparison {
  name: string;
  feature: string;
  dataSize: number;
  baselineOps: number;
  currentOps: number;
  changePercent: number;
  status: 'improvement' | 'stable' | 'regression';
}

// Export for use
export { PerformanceTracker, PerformanceSnapshot, BenchmarkResult, TrendData };

// CLI usage
if (require.main === module) {
  const tracker = new PerformanceTracker();

  const args = process.argv.slice(2);
  const command = args[0] || 'report';

  switch (command) {
    case 'report':
      console.log(tracker.generateReport());
      break;
    case 'history':
      const history = tracker.loadHistory();
      console.log(`Found ${history.length} historical snapshots`);
      for (const snapshot of history.slice(0, 5)) {
        console.log(`- ${snapshot.timestamp}: ${snapshot.results.length} tests`);
      }
      break;
    case 'trends':
      const trends = tracker.analyzeTrends('Baseline', 30);
      if (trends) {
        console.log(`Trend for ${trends.metric}: ${trends.trend}`);
        console.log(`Change: ${trends.changePercent.toFixed(1)}%`);
      }
      break;
    default:
      console.log('Usage: ts-node performance-tracker.ts [report|history|trends]');
  }
}
