/**
 * Browser Performance Comparison Report Generator
 * 
 * Analyzes benchmark results from Chrome and Firefox and generates comparison report
 */

import * as fs from 'fs';
import * as path from 'path';

interface BenchmarkResult {
  browser: string;
  browserVersion: string;
  testName: string;
  feature: string;
  dataSize: number;
  opsPerSecond: number;
  avgTime: number;
  minTime: number;
  maxTime: number;
  stdDev: number;
  memoryBefore?: number;
  memoryAfter?: number;
}

interface ComparisonEntry {
  testName: string;
  feature: string;
  dataSize: number;
  chromeOps: number;
  firefoxOps: number;
  chromeVersion: string;
  firefoxVersion: string;
  speedup: number; // Chrome/Firefox ratio, >1 means Chrome is faster
  winner: 'Chrome' | 'Firefox' | 'Tie';
  difference: number; // Percentage difference
}

export class BrowserComparisonReport {
  private results: BenchmarkResult[] = [];

  addResult(result: BenchmarkResult): void {
    this.results.push(result);
  }

  generateComparison(): ComparisonEntry[] {
    const comparisons: ComparisonEntry[] = [];
    
    // Group by test name and data size
    const grouped = new Map<string, BenchmarkResult[]>();
    
    for (const result of this.results) {
      const key = `${result.testName}-${result.dataSize}`;
      const existing = grouped.get(key) || [];
      existing.push(result);
      grouped.set(key, existing);
    }

    // Compare Chrome vs Firefox for each test
    for (const [key, results] of grouped) {
      if (results.length >= 2) {
        const chrome = results.find(r => r.browser === 'Chrome');
        const firefox = results.find(r => r.browser === 'Firefox');
        
        if (chrome && firefox) {
          const speedup = chrome.opsPerSecond / firefox.opsPerSecond;
          const winner = speedup > 1.05 ? 'Chrome' : speedup < 0.95 ? 'Firefox' : 'Tie';
          const difference = Math.abs(speedup - 1) * 100;
          
          comparisons.push({
            testName: chrome.testName,
            feature: chrome.feature,
            dataSize: chrome.dataSize,
            chromeOps: chrome.opsPerSecond,
            firefoxOps: firefox.opsPerSecond,
            chromeVersion: chrome.browserVersion,
            firefoxVersion: firefox.browserVersion,
            speedup,
            winner,
            difference,
          });
        }
      }
    }

    return comparisons.sort((a, b) => Math.abs(b.difference) - Math.abs(a.difference));
  }

  generateMarkdownReport(): string {
    const comparisons = this.generateComparison();
    
    if (comparisons.length === 0) {
      return '# Browser Performance Comparison\n\nNo comparison data available.';
    }

    const chromeVersion = comparisons[0]?.chromeVersion || 'unknown';
    const firefoxVersion = comparisons[0]?.firefoxVersion || 'unknown';

    let report = '# XPB Browser Performance Comparison\n\n';
    report += `Generated: ${new Date().toISOString()}\n\n`;
    
    report += '## Test Environment\n\n';
    report += `- **Chrome**: ${chromeVersion}\n`;
    report += `- **Firefox**: ${firefoxVersion}\n`;
    report += `- **Test Date**: ${new Date().toLocaleDateString()}\n\n`;

    // Summary statistics
    const chromeWins = comparisons.filter(c => c.winner === 'Chrome').length;
    const firefoxWins = comparisons.filter(c => c.winner === 'Firefox').length;
    const ties = comparisons.filter(c => c.winner === 'Tie').length;
    
    report += '## Summary\n\n';
    report += `| Metric | Count |\n`;
    report += `|--------|-------|\n`;
    report += `| Total Tests | ${comparisons.length} |\n`;
    report += `| Chrome Wins | ${chromeWins} |\n`;
    report += `| Firefox Wins | ${firefoxWins} |\n`;
    report += `| Ties | ${ties} |\n\n`;

    // Average performance by browser
    const avgChromeSpeedup = comparisons.reduce((sum, c) => sum + c.speedup, 0) / comparisons.length;
    report += `**Average Performance**: Chrome is ${avgChromeSpeedup.toFixed(2)}x of Firefox\n\n`;

    // Detailed results table
    report += '## Detailed Results\n\n';
    report += '| Test | Data Size | Chrome (ops/sec) | Firefox (ops/sec) | Winner | Difference |\n';
    report += '|------|-----------|------------------|-------------------|--------|------------|\n';
    
    for (const comp of comparisons) {
      const chromeStr = comp.chromeOps.toFixed(0);
      const firefoxStr = comp.firefoxOps.toFixed(0);
      const diffStr = `${comp.difference.toFixed(1)}%`;
      const winnerIcon = comp.winner === 'Chrome' ? '🟦' : comp.winner === 'Firefox' ? '🟧' : '⬜';
      
      report += `| ${comp.testName} | ${comp.dataSize}B | ${chromeStr} | ${firefoxStr} | ${winnerIcon} ${comp.winner} | ${diffStr} |\n`;
    }

    // Best performers
    report += '\n## Best Performers by Test\n\n';
    
    const chromeAdvantage = comparisons.filter(c => c.winner === 'Chrome');
    const firefoxAdvantage = comparisons.filter(c => c.winner === 'Firefox');
    
    if (chromeAdvantage.length > 0) {
      report += '### Chrome is Faster\n\n';
      chromeAdvantage.slice(0, 5).forEach(c => {
        report += `- **${c.testName}** (${c.dataSize}B): ${c.speedup.toFixed(2)}x faster\n`;
      });
      report += '\n';
    }
    
    if (firefoxAdvantage.length > 0) {
      report += '### Firefox is Faster\n\n';
      firefoxAdvantage.slice(0, 5).forEach(c => {
        report += `- **${c.testName}** (${c.dataSize}B): ${(1/c.speedup).toFixed(2)}x faster\n`;
      });
      report += '\n';
    }

    // Recommendations
    report += '## Recommendations\n\n';
    
    if (avgChromeSpeedup > 1.2) {
      report += '- **Chrome is significantly faster overall**. Consider recommending Chrome for XPB-heavy applications.\n';
    } else if (avgChromeSpeedup < 0.8) {
      report += '- **Firefox is significantly faster overall**. Consider recommending Firefox for XPB-heavy applications.\n';
    } else {
      report += '- **Both browsers perform similarly**. Choose based on other factors (memory usage, compatibility, etc.).\n';
    }
    
    report += '\n### Feature-Specific Recommendations\n\n';
    
    // Group by feature
    const byFeature = new Map<string, ComparisonEntry[]>();
    for (const comp of comparisons) {
      const existing = byFeature.get(comp.feature) || [];
      existing.push(comp);
      byFeature.set(comp.feature, existing);
    }
    
    for (const [feature, comps] of byFeature) {
      const avgSpeedup = comps.reduce((sum, c) => sum + c.speedup, 0) / comps.length;
      const winner = avgSpeedup > 1.1 ? 'Chrome' : avgSpeedup < 0.9 ? 'Firefox' : 'similar';
      
      report += `- **${feature || 'baseline'}**: ${winner === 'similar' ? 'Performance is similar' : `${winner} is ${Math.max(avgSpeedup, 1/avgSpeedup).toFixed(2)}x faster`}\n`;
    }

    // Raw data
    report += '\n## Raw Data\n\n';
    report += '```json\n';
    report += JSON.stringify(comparisons, null, 2);
    report += '\n```\n';

    return report;
  }

  saveReport(outputPath: string): void {
    const report = this.generateMarkdownReport();
    fs.writeFileSync(outputPath, report);
    console.log(`✓ Browser comparison report saved: ${outputPath}`);
  }

  static fromJSONFile(filePath: string): BrowserComparisonReport {
    const data = JSON.parse(fs.readFileSync(filePath, 'utf-8'));
    const report = new BrowserComparisonReport();
    
    // Handle Playwright JSON format
    if (data.suites) {
      for (const suite of data.suites) {
        for (const spec of suite.specs || []);
        for (const test of spec.tests || []) {
          for (const result of test.results || []) {
            for (const attachment of result.attachments || []) {
              if (attachment.contentType === 'application/json' && attachment.body) {
                try {
                  const benchmarkResult = JSON.parse(Buffer.from(attachment.body, 'base64').toString());
                  report.addResult(benchmarkResult);
                } catch (e) {
                  // Skip invalid attachments
                }
              }
            }
          }
        }
      }
    }
    
    return report;
  }
}

// CLI usage
if (require.main === module) {
  const args = process.argv.slice(2);
  const command = args[0] || 'generate';
  
  switch (command) {
    case 'generate':
      const inputFile = args[1] || 'benchmarks/results/browser-results.json';
      const outputFile = args[2] || 'benchmarks/results/browser-comparison.md';
      
      if (fs.existsSync(inputFile)) {
        const report = BrowserComparisonReport.fromJSONFile(inputFile);
        report.saveReport(outputFile);
      } else {
        console.error(`Input file not found: ${inputFile}`);
        process.exit(1);
      }
      break;
      
    default:
      console.log('Usage: ts-node browser-comparison.ts [generate] [input.json] [output.md]');
  }
}

export default BrowserComparisonReport;
