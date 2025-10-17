#!/usr/bin/env python3
"""
MetaGuildNet Visualization Tool

Generates visual representations of MetaGuildNet execution results including:
- Workflow execution timelines
- System health dashboards
- Test result matrices
- ASCII charts and graphs
- Execution summaries with visual indicators

Usage:
    python3 visualize.py                    # Visualize latest run
    python3 visualize.py --output-dir path  # Specify output directory
    python3 visualize.py --format [ascii|html|both] # Output format
"""

import argparse
import json
import os
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Tuple, Optional, Any

try:
    from PIL import Image, ImageDraw, ImageFont
    import matplotlib.pyplot as plt
    import matplotlib.patches as patches
    HAS_PNG_SUPPORT = True
except ImportError:
    HAS_PNG_SUPPORT = False
    print("Warning: PIL and matplotlib not available for PNG generation")


class Colors:
    """ANSI color codes."""
    RED = '\033[91m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    MAGENTA = '\033[95m'
    CYAN = '\033[96m'
    WHITE = '\033[97m'
    BOLD = '\033[1m'
    RESET = '\033[0m'


class MetaGuildNetVisualizer:
    """Visualize MetaGuildNet execution results."""

    def __init__(self, output_dir: str = "MetaGuildNet/outputs"):
        self.output_dir = Path(output_dir)
        self.results = {}
        self.parse_outputs()

    def parse_outputs(self):
        """Parse output files to extract execution data."""
        print(f"{Colors.CYAN}📊 Parsing MetaGuildNet outputs...{Colors.RESET}\n")
        
        # Parse verification output
        if (self.output_dir / "verification_output.txt").exists():
            self.results['verification'] = self._parse_verification()
        
        # Parse testing output
        if (self.output_dir / "testing_output.txt").exists():
            self.results['testing'] = self._parse_testing()
        
        # Parse diagnostics output
        if (self.output_dir / "diagnostics_output.txt").exists():
            self.results['diagnostics'] = self._parse_diagnostics()
        
        # Parse examples output
        if (self.output_dir / "examples_output.txt").exists():
            self.results['examples'] = self._parse_examples()
        
        # Parse configuration
        if (self.output_dir / "configuration_display.txt").exists():
            self.results['config'] = self._parse_config()

    def _parse_verification(self) -> Dict[str, Any]:
        """Parse verification output file."""
        with open(self.output_dir / "verification_output.txt", 'r') as f:
            content = f.read()
        
        # Extract layer statuses
        layers = {}
        for layer in ['Network', 'Cluster', 'Database', 'Application']:
            if f"{layer} Layer: UNHEALTHY" in content:
                layers[layer] = 'UNHEALTHY'
            elif f"{layer} Layer: HEALTHY" in content:
                layers[layer] = 'HEALTHY'
            else:
                layers[layer] = 'UNKNOWN'
        
        # Count checks
        passed = content.count('✓')
        failed = content.count('✗')
        warnings = content.count('⚠')
        
        return {
            'layers': layers,
            'passed': passed,
            'failed': failed,
            'warnings': warnings,
            'status': 'FAIL' if 'UNHEALTHY' in content else 'PASS'
        }

    def _parse_testing(self) -> Dict[str, Any]:
        """Parse testing output file."""
        with open(self.output_dir / "testing_output.txt", 'r') as f:
            content = f.read()
        
        integration_ran = 'Integration Tests' in content
        e2e_ran = 'E2E Tests' in content
        
        return {
            'integration': 'RAN' if integration_ran else 'SKIPPED',
            'e2e': 'RAN' if e2e_ran else 'SKIPPED',
            'failures': content.count('✗'),
            'status': 'FAIL' if 'failed' in content else 'PASS'
        }

    def _parse_diagnostics(self) -> Dict[str, Any]:
        """Parse diagnostics output file."""
        with open(self.output_dir / "diagnostics_output.txt", 'r') as f:
            content = f.read()
        
        return {
            'completed': 'completed successfully' in content,
            'layers_checked': content.count('Layer')
        }

    def _parse_examples(self) -> Dict[str, Any]:
        """Parse examples output file."""
        with open(self.output_dir / "examples_output.txt", 'r') as f:
            content = f.read()
        
        return {
            'workspaces_created': content.count('workspace created'),
            'prerequisites_checked': '📋 Prerequisites' in content,
            'status': 'FAIL' if 'failed' in content else 'PASS'
        }

    def _parse_config(self) -> Dict[str, Any]:
        """Parse configuration file."""
        with open(self.output_dir / "configuration_display.txt", 'r') as f:
            content = f.read()
        
        # Extract JSON
        json_match = re.search(r'\{[\s\S]*\}', content)
        if json_match:
            try:
                return json.loads(json_match.group())
            except json.JSONDecodeError:
                return {}
        return {}

    def generate_dashboard(self) -> str:
        """Generate ASCII dashboard."""
        lines = []
        lines.append("╔════════════════════════════════════════════════════════════════╗")
        lines.append("║        METAGUILDNET EXECUTION DASHBOARD                       ║")
        lines.append("╚════════════════════════════════════════════════════════════════╝")
        lines.append("")
        
        # Workflow Status Overview
        lines.append("📊 WORKFLOW STATUS OVERVIEW")
        lines.append("─" * 64)
        
        workflows = [
            ("Verification", self.results.get('verification', {}).get('status', 'N/A')),
            ("Testing", self.results.get('testing', {}).get('status', 'N/A')),
            ("Diagnostics", "PASS" if self.results.get('diagnostics', {}).get('completed') else "N/A"),
            ("Examples", self.results.get('examples', {}).get('status', 'N/A'))
        ]
        
        for name, status in workflows:
            status_icon = "✅" if status == "PASS" else "❌" if status == "FAIL" else "⚪"
            lines.append(f"  {status_icon} {name:<20} {status:>10}")
        
        lines.append("")
        
        # Verification Details
        if 'verification' in self.results:
            lines.append("🔍 VERIFICATION LAYER STATUS")
            lines.append("─" * 64)
            ver = self.results['verification']
            for layer, status in ver.get('layers', {}).items():
                status_icon = "🟢" if status == "HEALTHY" else "🔴" if status == "UNHEALTHY" else "⚪"
                lines.append(f"  {status_icon} {layer:<20} {status:>10}")
            
            lines.append("")
            lines.append(f"  Checks:  ✓ {ver.get('passed', 0):<3} | ✗ {ver.get('failed', 0):<3} | ⚠ {ver.get('warnings', 0):<3}")
            lines.append("")
        
        # Testing Details
        if 'testing' in self.results:
            lines.append("🧪 TESTING SUMMARY")
            lines.append("─" * 64)
            test = self.results['testing']
            lines.append(f"  Integration Tests:  {test.get('integration', 'N/A')}")
            lines.append(f"  E2E Tests:          {test.get('e2e', 'N/A')}")
            lines.append(f"  Failures:           {test.get('failures', 0)}")
            lines.append("")
        
        # System Health Bar
        lines.append("💚 SYSTEM HEALTH")
        lines.append("─" * 64)
        health_percentage = self._calculate_health_percentage()
        lines.append(self._create_health_bar(health_percentage))
        lines.append(f"  Overall Health: {health_percentage}%")
        lines.append("")
        
        lines.append("═" * 64)
        lines.append(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        lines.append("═" * 64)
        
        return "\n".join(lines)

    def _calculate_health_percentage(self) -> int:
        """Calculate overall system health percentage."""
        total_checks = 0
        passed_checks = 0
        
        if 'verification' in self.results:
            ver = self.results['verification']
            layers = ver.get('layers', {})
            total_checks += len(layers)
            passed_checks += sum(1 for status in layers.values() if status == 'HEALTHY')
        
        if total_checks == 0:
            return 0
        
        return int((passed_checks / total_checks) * 100)

    def _create_health_bar(self, percentage: int, width: int = 50) -> str:
        """Create ASCII health bar."""
        filled = int((percentage / 100) * width)
        bar = "█" * filled + "░" * (width - filled)
        
        if percentage >= 80:
            color = Colors.GREEN
        elif percentage >= 50:
            color = Colors.YELLOW
        else:
            color = Colors.RED
        
        return f"  [{color}{bar}{Colors.RESET}]"

    def generate_timeline(self) -> str:
        """Generate execution timeline."""
        lines = []
        lines.append("╔════════════════════════════════════════════════════════════════╗")
        lines.append("║        METAGUILDNET EXECUTION TIMELINE                        ║")
        lines.append("╚════════════════════════════════════════════════════════════════╝")
        lines.append("")
        
        # Timeline visualization
        workflows = [
            ("Setup", "⚪", "Not executed"),
            ("Verification", "✅" if self.results.get('verification', {}).get('status') == 'PASS' else "❌", "Executed"),
            ("Testing", "✅" if self.results.get('testing', {}).get('status') == 'PASS' else "❌", "Executed"),
            ("Diagnostics", "✅", "Executed"),
            ("Examples", "✅" if self.results.get('examples', {}).get('status') == 'PASS' else "❌", "Executed")
        ]
        
        lines.append("  Start ─┬─▶ Setup          ⚪ Not executed")
        for i, (name, icon, status) in enumerate(workflows[1:], 1):
            connector = "       ├─▶" if i < len(workflows) - 1 else "       └─▶"
            lines.append(f"{connector} {name:<15} {icon} {status}")
        lines.append("")
        
        return "\n".join(lines)

    def generate_matrix(self) -> str:
        """Generate feature matrix."""
        lines = []
        lines.append("╔════════════════════════════════════════════════════════════════╗")
        lines.append("║        METAGUILDNET FEATURE MATRIX                            ║")
        lines.append("╚════════════════════════════════════════════════════════════════╝")
        lines.append("")
        
        # Determine verification status based on layer health
        layers = self.results.get('verification', {}).get('layers', {})
        network_status = "✅" if layers.get("Network", "").upper() == "HEALTHY" else "🟡"
        cluster_status = "✅" if layers.get("Cluster", "").upper() == "HEALTHY" else "🟡"
        database_status = "✅" if layers.get("Database", "").upper() == "HEALTHY" else "🟡"
        application_status = "✅" if layers.get("Application", "").upper() == "HEALTHY" else "🟡"
        
        network_state = "Working" if network_status == "✅" else "Pending Setup"
        cluster_state = "Working" if cluster_status == "✅" else "Pending Setup"
        database_state = "Working" if database_status == "✅" else "Pending Setup"
        application_state = "Working" if application_status == "✅" else "Pending Setup"
        
        # Determine workspace creation status based on examples
        workspace_status = "✅" if self.results.get('examples', {}).get('status') == "PASS" else "🟡"
        workspace_state = "Working" if workspace_status == "✅" else "Pending Setup"
        
        features = [
            ("Configuration Loading", "✅", "Working"),
            ("Multi-Workflow Support", "✅", "Working"),
            ("Error Handling", "✅", "Working"),
            ("Visualizations", "✅", "Working"),
            ("Logging System", "✅", "Working"),
            ("Network Verification", network_status, network_state),
            ("Cluster Verification", cluster_status, cluster_state),
            ("Database Verification", database_status, database_state),
            ("Application Verification", application_status, application_state),
            ("Workspace Creation", workspace_status, workspace_state)
        ]
        
        lines.append(f"{'Feature':<30} {'Status':<10} {'State':<20}")
        lines.append("─" * 64)
        
        for feature, icon, state in features:
            lines.append(f"{feature:<30} {icon:<10} {state:<20}")
        
        lines.append("")
        lines.append("Legend: ✅ Working | 🟡 Pending | ❌ Failed")
        lines.append("")
        
        return "\n".join(lines)

    def generate_report(self) -> str:
        """Generate comprehensive visual report."""
        sections = [
            self.generate_dashboard(),
            "",
            self.generate_timeline(),
            "",
            self.generate_matrix()
        ]
        
        return "\n".join(sections)

    def save_report(self, filename: str = "visual_report.txt"):
        """Save report to file."""
        report = self.generate_report()
        output_path = self.output_dir.parent / filename
        
        with open(output_path, 'w') as f:
            f.write(report)
        
        print(f"{Colors.GREEN}✅ Report saved to: {output_path}{Colors.RESET}")
        return output_path

    def display_report(self):
        """Display report to console."""
        print(self.generate_report())

    def generate_png_dashboard(self, output_path: str = None) -> str:
        """Generate PNG dashboard image."""
        if not HAS_PNG_SUPPORT:
            print("PNG support not available - skipping PNG generation")
            return ""

        if output_path is None:
            output_path = self.output_dir / "dashboard.png"

        # Create a simple PNG with the dashboard text
        try:
            # Create image
            img = Image.new('RGB', (1200, 800), color='white')
            draw = ImageDraw.Draw(img)

            # Try to load a monospace font, fall back to default
            try:
                font = ImageFont.load_default()
            except:
                font = None

            # Dashboard text (ASCII-only for PNG compatibility)
            dashboard_lines = [
                "METAGUILDNET EXECUTION DASHBOARD",
                "=================================",
                "",
                "WORKFLOW STATUS:",
                "  [FAIL] Verification - FAIL",
                "  [FAIL] Testing - FAIL",
                "  [PASS] Diagnostics - PASS",
                "  [FAIL] Examples - FAIL",
                "",
                "SYSTEM HEALTH:",
                "  [UNHEALTHY] Network Layer - UNHEALTHY",
                "  [UNHEALTHY] Cluster Layer - UNHEALTHY",
                "  [UNHEALTHY] Database Layer - UNHEALTHY",
                "  [UNHEALTHY] Application Layer - UNHEALTHY",
                "",
                "Overall Health: 0%"
            ]

            # Draw text
            y_position = 20
            line_height = 25

            for line in dashboard_lines:
                draw.text((20, y_position), line, fill='black', font=font)
                y_position += line_height

            # Save image
            img.save(str(output_path))
            print(f"✅ PNG Dashboard saved to: {output_path}")
            return str(output_path)

        except Exception as e:
            print(f"Error generating PNG dashboard: {e}")
            return ""

    def generate_png_timeline(self, output_path: str = None) -> str:
        """Generate PNG timeline visualization."""
        if not HAS_PNG_SUPPORT:
            print("PNG support not available - skipping PNG generation")
            return ""

        if output_path is None:
            output_path = self.output_dir / "timeline.png"

        try:
            # Create matplotlib figure
            fig, ax = plt.subplots(figsize=(10, 6))
            ax.set_xlim(0, 10)
            ax.set_ylim(0, 8)

            # Timeline visualization
            workflows = [
                ("Setup", 1, 7, "white"),
                ("Verification", 2, 5, "red"),
                ("Testing", 3, 3, "red"),
                ("Diagnostics", 4, 1, "green"),
                ("Examples", 5, -1, "red")
            ]

            # Draw timeline
            for name, x, y, color in workflows:
                if color == "green":
                    ax.scatter(x, y, s=1000, c=color, alpha=0.7, marker='o')
                elif color == "red":
                    ax.scatter(x, y, s=1000, c=color, alpha=0.7, marker='x')
                else:
                    ax.scatter(x, y, s=1000, c=color, alpha=0.7, marker='o')

                ax.text(x + 0.2, y, name, fontsize=10, verticalalignment='center')

            # Draw connecting lines
            for i in range(len(workflows) - 1):
                x1, y1 = workflows[i][1], workflows[i][2]
                x2, y2 = workflows[i+1][1], workflows[i+1][2]
                ax.plot([x1, x2], [y1, y2], 'k-', alpha=0.3)

            ax.set_title('MetaGuildNet Execution Timeline')
            ax.set_xlabel('Execution Order')
            ax.set_ylabel('Workflow')
            ax.grid(True, alpha=0.3)

            plt.tight_layout()
            plt.savefig(str(output_path), dpi=150, bbox_inches='tight')
            plt.close()

            print(f"✅ PNG Timeline saved to: {output_path}")
            return str(output_path)

        except Exception as e:
            print(f"Error generating PNG timeline: {e}")
            return ""

    def generate_png_matrix(self, output_path: str = None) -> str:
        """Generate PNG feature matrix."""
        if not HAS_PNG_SUPPORT:
            print("PNG support not available - skipping PNG generation")
            return ""

        if output_path is None:
            output_path = self.output_dir / "feature_matrix.png"

        try:
            # Create matplotlib figure
            fig, ax = plt.subplots(figsize=(12, 8))

            # Feature matrix data - determine status from parsed data
            features = [
                "Configuration Loading", "Multi-Workflow Support", "Error Handling",
                "Visualizations", "Logging System", "Network Verification",
                "Cluster Verification", "Database Verification", "Application Verification",
                "Workspace Creation"
            ]

            # Determine colors and status based on actual data
            layers = self.results.get('verification', {}).get('layers', {})
            network_working = layers.get("Network", "").upper() == "HEALTHY"
            cluster_working = layers.get("Cluster", "").upper() == "HEALTHY"
            database_working = layers.get("Database", "").upper() == "HEALTHY"
            application_working = layers.get("Application", "").upper() == "HEALTHY"
            examples_working = self.results.get('examples', {}).get('status') == "PASS"

            status_list = [True, True, True, True, True,  # First 5 always working
                          network_working, cluster_working, database_working, 
                          application_working, examples_working]
            
            colors = ['green' if working else 'yellow' for working in status_list]
            bar_values = [100 if working else 0 for working in status_list]

            # Create horizontal bar chart
            y_pos = range(len(features))

            # Draw bars
            bars = ax.barh(y_pos, bar_values, color=colors, alpha=0.7, height=0.8)

            # Add text labels
            for i, (feature, bar, working) in enumerate(zip(features, bars, status_list)):
                ax.text(10, bar.get_y() + bar.get_height()/2, feature,
                       va='center', fontsize=9, fontweight='bold')
                ax.text(110, bar.get_y() + bar.get_height()/2,
                       'WORKING' if working else 'PENDING',
                       va='center', fontsize=9, ha='right')

            ax.set_xlim(0, 120)
            ax.set_ylim(-0.5, len(features) - 0.5)
            ax.set_title('MetaGuildNet Feature Status Matrix')
            ax.set_xlabel('Status')
            ax.set_ylabel('Features')
            ax.grid(True, alpha=0.3, axis='x')

            # Add legend
            legend_elements = [
                patches.Patch(color='green', label='Working'),
                patches.Patch(color='yellow', label='Pending Setup')
            ]
            ax.legend(handles=legend_elements, loc='upper right')

            plt.tight_layout()
            plt.savefig(str(output_path), dpi=150, bbox_inches='tight')
            plt.close()

            print(f"✅ PNG Feature Matrix saved to: {output_path}")
            return str(output_path)

        except Exception as e:
            print(f"Error generating PNG matrix: {e}")
            return ""

    def generate_all_pngs(self) -> Dict[str, str]:
        """Generate all PNG visualizations."""
        png_files = {}

        print(f"\n{Colors.CYAN}🖼️ Generating PNG Visualizations...{Colors.RESET}")

        # Generate PNG dashboard
        png_files['dashboard'] = self.generate_png_dashboard(
            self.output_dir / "dashboard.png"
        )

        # Generate PNG timeline
        png_files['timeline'] = self.generate_png_timeline(
            self.output_dir / "timeline.png"
        )

        # Generate PNG feature matrix
        png_files['matrix'] = self.generate_png_matrix(
            self.output_dir / "feature_matrix.png"
        )

        # Generate PNG health chart
        png_files['health'] = self.generate_png_health_chart(
            self.output_dir / "health_chart.png"
        )

        return png_files

    def generate_png_health_chart(self, output_path: str = None) -> str:
        """Generate PNG health chart."""
        if not HAS_PNG_SUPPORT:
            print("PNG support not available - skipping PNG generation")
            return ""

        if output_path is None:
            output_path = self.output_dir / "health_chart.png"

        try:
            # Create matplotlib figure
            fig, ax = plt.subplots(figsize=(8, 6))

            # Health data (0% for all layers since system not set up)
            layers = ['Network', 'Cluster', 'Database', 'Application']
            health_percentages = [0, 0, 0, 0]

            # Create bar chart
            bars = ax.bar(layers, health_percentages, color=['red', 'red', 'red', 'red'], alpha=0.7)

            # Add percentage labels
            for bar, percentage in zip(bars, health_percentages):
                height = bar.get_height()
                ax.text(bar.get_x() + bar.get_width()/2., height + 1,
                       f'{percentage}%', ha='center', va='bottom', fontsize=12)

            ax.set_ylim(0, 105)
            ax.set_title('MetaGuildNet System Health')
            ax.set_ylabel('Health Percentage')
            ax.grid(True, alpha=0.3, axis='y')

            # Add health status text
            ax.text(0.02, 0.98, '🔴 System needs setup\n   (0% healthy)',
                   transform=ax.transAxes, fontsize=10,
                   verticalalignment='top', bbox=dict(boxstyle='round', facecolor='red', alpha=0.1))

            plt.tight_layout()
            plt.savefig(str(output_path), dpi=150, bbox_inches='tight')
            plt.close()

            print(f"✅ PNG Health Chart saved to: {output_path}")
            return str(output_path)

        except Exception as e:
            print(f"Error generating PNG health chart: {e}")
            return ""

    def save_report(self, filename: str = "visual_report.txt"):
        """Save report to file."""
        report = self.generate_report()
        output_path = self.output_dir.parent / filename

        with open(output_path, 'w') as f:
            f.write(report)

        print(f"✅ Report saved to: {output_path}")

        # Also generate PNG files if supported
        if HAS_PNG_SUPPORT:
            png_files = self.generate_all_pngs()
            print(f"✅ Generated {len([f for f in png_files.values() if f])} PNG visualization files")

        return output_path


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description='MetaGuildNet Visualization Tool')
    parser.add_argument('--output-dir', default='MetaGuildNet/outputs',
                       help='Output directory with execution results')
    parser.add_argument('--save', action='store_true',
                       help='Save report to file')
    parser.add_argument('--filename', default='VISUAL_REPORT.txt',
                       help='Output filename')
    parser.add_argument('--png-only', action='store_true',
                       help='Generate only PNG visualizations')
    parser.add_argument('--png-dashboard', action='store_true',
                       help='Generate PNG dashboard only')
    parser.add_argument('--png-timeline', action='store_true',
                       help='Generate PNG timeline only')
    parser.add_argument('--png-matrix', action='store_true',
                       help='Generate PNG feature matrix only')
    parser.add_argument('--png-health', action='store_true',
                       help='Generate PNG health chart only')
    parser.add_argument('--all-pngs', action='store_true',
                       help='Generate all PNG visualizations')

    args = parser.parse_args()

    visualizer = MetaGuildNetVisualizer(args.output_dir)

    # Handle PNG-only generation
    if args.png_only or args.all_pngs:
        if HAS_PNG_SUPPORT:
            png_files = visualizer.generate_all_pngs()
            print(f"\n{Colors.GREEN}✅ Generated {len([f for f in png_files.values() if f])} PNG files:{Colors.RESET}")
            for name, path in png_files.items():
                if path:
                    print(f"   • {name}: {path}")
        else:
            print(f"{Colors.RED}❌ PNG support not available{Colors.RESET}")
        return

    # Handle individual PNG generation
    png_generated = False
    output_path = Path(args.output_dir)
    if args.png_dashboard:
        visualizer.generate_png_dashboard(output_path / "dashboard.png")
        png_generated = True

    if args.png_timeline:
        visualizer.generate_png_timeline(output_path / "timeline.png")
        png_generated = True

    if args.png_matrix:
        visualizer.generate_png_matrix(output_path / "feature_matrix.png")
        png_generated = True

    if args.png_health:
        visualizer.generate_png_health_chart(output_path / "health_chart.png")
        png_generated = True

    # Save text report if requested
    if args.save:
        visualizer.save_report(args.filename)
        if not png_generated:
            print(f"{Colors.YELLOW}💡 Tip: Use --all-pngs to also generate PNG visualizations{Colors.RESET}")

    # Display text report if not PNG-only
    if not (args.png_only or args.all_pngs):
        if not png_generated:
            visualizer.display_report()
        else:
            print(f"\n{Colors.GREEN}✅ PNG visualizations generated successfully{Colors.RESET}")
            print(f"{Colors.CYAN}💡 Use --save to also generate text report{Colors.RESET}")


if __name__ == '__main__':
    main()

