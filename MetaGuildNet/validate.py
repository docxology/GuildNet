#!/usr/bin/env python3
"""
MetaGuildNet Validation Tool

Comprehensive validation of MetaGuildNet functionality including:
- Output file integrity checks
- Method validation
- Configuration validation
- Performance benchmarking
- Regression testing

Usage:
    python3 validate.py                 # Run all validations
    python3 validate.py --quick         # Quick validation only
    python3 validate.py --benchmark     # Include performance tests
"""

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, List, Tuple, Optional


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


class MetaGuildNetValidator:
    """Validate MetaGuildNet functionality."""

    def __init__(self, project_root: Path = None):
        self.project_root = project_root or Path.cwd()
        self.meta_dir = self.project_root / "MetaGuildNet"
        self.results = []
        self.passed = 0
        self.failed = 0

    def _test(self, name: str, func, *args, **kwargs) -> bool:
        """Run a test and record result."""
        print(f"  Testing: {name}...", end=" ")
        try:
            result = func(*args, **kwargs)
            if result:
                print(f"{Colors.GREEN}✅ PASS{Colors.RESET}")
                self.passed += 1
                self.results.append((name, True, None))
                return True
            else:
                print(f"{Colors.RED}❌ FAIL{Colors.RESET}")
                self.failed += 1
                self.results.append((name, False, "Assertion failed"))
                return False
        except Exception as e:
            print(f"{Colors.RED}❌ FAIL{Colors.RESET} - {str(e)}")
            self.failed += 1
            self.results.append((name, False, str(e)))
            return False

    def validate_file_structure(self) -> bool:
        """Validate MetaGuildNet directory structure."""
        print(f"\n{Colors.CYAN}📁 Validating File Structure{Colors.RESET}")
        
        required_files = [
            "run.py",
            "config.json",
            "README.md",
            "Makefile"
        ]
        
        required_dirs = [
            "scripts",
            "tests",
            "docs",
            "examples",
            "outputs",
            "reports"
        ]
        
        all_pass = True
        
        for file in required_files:
            all_pass &= self._test(
                f"File exists: {file}",
                lambda f: (self.meta_dir / f).exists(),
                file
            )
        
        for dir in required_dirs:
            all_pass &= self._test(
                f"Directory exists: {dir}",
                lambda d: (self.meta_dir / d).exists(),
                dir
            )
        
        return all_pass

    def validate_configuration(self) -> bool:
        """Validate configuration files."""
        print(f"\n{Colors.CYAN}⚙️ Validating Configuration{Colors.RESET}")
        
        config_file = self.meta_dir / "config.json"
        
        self._test(
            "Config file exists",
            lambda: config_file.exists()
        )
        
        def validate_json():
            with open(config_file, 'r') as f:
                config = json.load(f)
            return isinstance(config, dict) and len(config) > 0
        
        self._test(
            "Config is valid JSON",
            validate_json
        )
        
        def validate_sections():
            with open(config_file, 'r') as f:
                config = json.load(f)
            required_sections = ['meta_setup', 'verification', 'testing', 'examples', 'diagnostics', 'logging']
            return all(section in config for section in required_sections)
        
        self._test(
            "Config has required sections",
            validate_sections
        )
        
        return True

    def validate_outputs(self) -> bool:
        """Validate output files."""
        print(f"\n{Colors.CYAN}📊 Validating Output Files{Colors.RESET}")
        
        outputs_dir = self.meta_dir / "outputs"
        
        expected_outputs = [
            "verification_output.txt",
            "diagnostics_output.txt",
            "testing_output.txt",
            "examples_output.txt",
            "configuration_display.txt",
            "help_display.txt"
        ]
        
        for output in expected_outputs:
            output_path = outputs_dir / output
            
            self._test(
                f"Output exists: {output}",
                lambda p: p.exists(),
                output_path
            )
            
            def check_size(p):
                return p.exists() and p.stat().st_size > 0
            
            self._test(
                f"Output not empty: {output}",
                check_size,
                output_path
            )
            
            def check_content(p):
                if not p.exists():
                    return False
                with open(p, 'r') as f:
                    content = f.read()
                return '╔' in content or 'MetaGuildNet' in content
            
            self._test(
                f"Output has content: {output}",
                check_content,
                output_path
            )
        
        return True

    def validate_visualizations(self) -> bool:
        """Validate visualization elements in outputs."""
        print(f"\n{Colors.CYAN}🎨 Validating Visualizations{Colors.RESET}")
        
        outputs_dir = self.meta_dir / "outputs"
        verification_output = outputs_dir / "verification_output.txt"
        
        if not verification_output.exists():
            print(f"  {Colors.YELLOW}⚠ Skipping - verification output not found{Colors.RESET}")
            return True
        
        with open(verification_output, 'r') as f:
            content = f.read()
        
        visualization_elements = [
            ('╔', 'Unicode box drawing (top left)'),
            ('═', 'Unicode box drawing (horizontal)'),
            ('╚', 'Unicode box drawing (bottom left)'),
            ('[96m', 'ANSI cyan color code'),
            ('[95m', 'ANSI magenta color code'),
            ('✗', 'Status indicator (failure)'),
            ('⚠', 'Status indicator (warning)'),
            ('[0m', 'ANSI reset code')
        ]
        
        for element, description in visualization_elements:
            self._test(
                f"Has {description}",
                lambda e, c=content: e in c,
                element
            )
        
        return True

    def validate_python_runner(self) -> bool:
        """Validate Python runner functionality."""
        print(f"\n{Colors.CYAN}🐍 Validating Python Runner{Colors.RESET}")
        
        run_py = self.meta_dir / "run.py"
        
        self._test(
            "run.py is executable or has shebang",
            lambda: run_py.exists() and (os.access(run_py, os.X_OK) or 
                                          open(run_py).readline().startswith('#!'))
        )
        
        def check_imports():
            with open(run_py, 'r') as f:
                content = f.read()
            required_imports = ['argparse', 'json', 'subprocess', 'pathlib']
            return all(f"import {imp}" in content for imp in required_imports)
        
        self._test(
            "Has required imports",
            check_imports
        )
        
        def check_classes():
            with open(run_py, 'r') as f:
                content = f.read()
            return 'class MetaGuildNetRunner' in content
        
        self._test(
            "Has MetaGuildNetRunner class",
            check_classes
        )
        
        # Test --help works
        def test_help():
            result = subprocess.run(
                ['python3', str(run_py), '--help'],
                capture_output=True,
                timeout=5
            )
            return result.returncode == 0 and b'MetaGuildNet' in result.stdout
        
        self._test(
            "Runner --help works",
            test_help
        )
        
        return True

    def validate_scripts(self) -> bool:
        """Validate shell scripts."""
        print(f"\n{Colors.CYAN}📜 Validating Shell Scripts{Colors.RESET}")
        
        scripts_dir = self.meta_dir / "scripts"
        
        def count_scripts():
            return sum(1 for _ in scripts_dir.rglob("*.sh"))
        
        self._test(
            "Has shell scripts",
            lambda: count_scripts() > 0
        )
        
        # Check a few key scripts
        key_scripts = [
            "scripts/setup/setup_wizard.sh",
            "scripts/verify/verify_all.sh",
            "scripts/utils/diagnose.sh"
        ]
        
        for script in key_scripts:
            script_path = self.meta_dir / script
            self._test(
                f"Script exists: {script}",
                lambda p: p.exists(),
                script_path
            )
        
        return True

    def validate_documentation(self) -> bool:
        """Validate documentation completeness."""
        print(f"\n{Colors.CYAN}📚 Validating Documentation{Colors.RESET}")
        
        docs = [
            ("README.md", ["MetaGuildNet", "Quick Start"]),
            ("docs/SETUP.md", ["Setup", "Installation"]),
            ("docs/VERIFICATION.md", ["Verification", "Health"]),
            ("QUICK_REFERENCE.md", ["Workflow", "Command"])
        ]
        
        for doc_file, required_keywords in docs:
            doc_path = self.meta_dir / doc_file
            
            self._test(
                f"Doc exists: {doc_file}",
                lambda p: p.exists(),
                doc_path
            )
            
            if doc_path.exists():
                def check_keywords(p, keywords):
                    with open(p, 'r') as f:
                        content = f.read()
                    return any(kw in content for kw in keywords)
                
                self._test(
                    f"Doc has content: {doc_file}",
                    check_keywords,
                    doc_path,
                    required_keywords
                )
        
        return True

    def benchmark_performance(self) -> bool:
        """Benchmark performance of key operations."""
        print(f"\n{Colors.CYAN}⚡ Benchmarking Performance{Colors.RESET}")
        
        run_py = self.meta_dir / "run.py"
        
        # Test dry-run performance
        def benchmark_dryrun():
            start = time.time()
            result = subprocess.run(
                ['python3', str(run_py), '--dry-run'],
                capture_output=True,
                timeout=10
            )
            elapsed = time.time() - start
            return result.returncode == 0 and elapsed < 5.0
        
        self._test(
            "Dry-run completes < 5s",
            benchmark_dryrun
        )
        
        # Test help performance
        def benchmark_help():
            start = time.time()
            result = subprocess.run(
                ['python3', str(run_py), '--help'],
                capture_output=True,
                timeout=5
            )
            elapsed = time.time() - start
            return result.returncode == 0 and elapsed < 2.0
        
        self._test(
            "Help completes < 2s",
            benchmark_help
        )
        
        return True

    def validate_reports(self) -> bool:
        """Validate report generation."""
        print(f"\n{Colors.CYAN}📋 Validating Reports{Colors.RESET}")
        
        reports_dir = self.meta_dir / "reports"
        
        expected_reports = [
            "EXECUTION_REPORT.md",
            "OUTPUT_SUMMARY.md"
        ]
        
        for report in expected_reports:
            report_path = reports_dir / report
            
            self._test(
                f"Report exists: {report}",
                lambda p: p.exists(),
                report_path
            )
            
            if report_path.exists():
                def check_size(p):
                    return p.stat().st_size > 1000  # At least 1KB
                
                self._test(
                    f"Report substantial: {report}",
                    check_size,
                    report_path
                )
        
        return True

    def run_all_validations(self, include_benchmark: bool = False) -> bool:
        """Run all validation tests."""
        print(f"\n{Colors.BOLD}{Colors.MAGENTA}╔════════════════════════════════════════════════════════════════╗{Colors.RESET}")
        print(f"{Colors.BOLD}{Colors.MAGENTA}║        METAGUILDNET COMPREHENSIVE VALIDATION               ║{Colors.RESET}")
        print(f"{Colors.BOLD}{Colors.MAGENTA}╚════════════════════════════════════════════════════════════════╝{Colors.RESET}")
        
        validations = [
            self.validate_file_structure,
            self.validate_configuration,
            self.validate_outputs,
            self.validate_visualizations,
            self.validate_python_runner,
            self.validate_scripts,
            self.validate_documentation,
            self.validate_reports
        ]
        
        if include_benchmark:
            validations.append(self.benchmark_performance)
        
        for validation in validations:
            validation()
        
        # Print summary
        print(f"\n{Colors.BOLD}{'═' * 64}{Colors.RESET}")
        print(f"{Colors.BOLD}VALIDATION SUMMARY{Colors.RESET}")
        print(f"{'═' * 64}")
        print(f"  {Colors.GREEN}Passed: {self.passed}{Colors.RESET}")
        print(f"  {Colors.RED}Failed: {self.failed}{Colors.RESET}")
        print(f"  Total:  {self.passed + self.failed}")
        
        if self.failed == 0:
            print(f"\n  {Colors.GREEN}{Colors.BOLD}✅ ALL VALIDATIONS PASSED{Colors.RESET}")
            success_rate = 100.0
        else:
            success_rate = (self.passed / (self.passed + self.failed)) * 100
            print(f"\n  {Colors.YELLOW}⚠ SOME VALIDATIONS FAILED{Colors.RESET}")
        
        print(f"  Success Rate: {success_rate:.1f}%")
        print(f"{'═' * 64}\n")
        
        return self.failed == 0


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description='MetaGuildNet Validation Tool')
    parser.add_argument('--quick', action='store_true',
                       help='Quick validation only')
    parser.add_argument('--benchmark', action='store_true',
                       help='Include performance benchmarks')
    
    args = parser.parse_args()
    
    validator = MetaGuildNetValidator()
    
    if args.quick:
        validator.validate_file_structure()
        validator.validate_configuration()
    else:
        success = validator.run_all_validations(include_benchmark=args.benchmark)
        sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()


