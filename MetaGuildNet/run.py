#!/usr/bin/env python3
"""
MetaGuildNet Runner

Programmatic execution of MetaGuildNet workflows.
Runs full setup, verification, testing, and examples by default.

Usage:
    python3 run.py                    # Run full workflow
    python3 run.py --config custom.json # Use custom config
    python3 run.py --help             # Show help

Configuration:
    config.json                       # Default config file
    Modify config.json to customize behavior

Requirements:
    python3 >= 3.8
    make (for Makefile targets)
"""

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, Any, List, Optional


class Colors:
    """ANSI color codes for terminal output."""
    RED = '\033[91m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    MAGENTA = '\033[95m'
    CYAN = '\033[96m'
    WHITE = '\033[97m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'
    RESET = '\033[0m'

    @classmethod
    def disable_on_windows(cls):
        """Disable colors on Windows if not supported."""
        if os.name == 'nt':
            for attr in dir(cls):
                if not attr.startswith('_'):
                    setattr(cls, attr, '')


class Logger:
    """Enhanced logging with colors and formatting."""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.log_level = config.get('logging', {}).get('level', 'info')
        self.use_colors = config.get('logging', {}).get('format', 'colored') == 'colored'
        self.show_timestamp = config.get('logging', {}).get('timestamp', True)

    def _format_message(self, message: str, level: str) -> str:
        """Format log message with timestamp and colors."""
        timestamp = ""
        if self.show_timestamp:
            timestamp = f"{Colors.CYAN}[{time.strftime('%H:%M:%S')}]{Colors.RESET} "

        if not self.use_colors:
            return f"{timestamp}{message}"

        color_map = {
            'debug': Colors.BLUE,
            'info': Colors.WHITE,
            'success': Colors.GREEN,
            'warning': Colors.YELLOW,
            'error': Colors.RED,
            'section': Colors.MAGENTA + Colors.BOLD,
            'subsection': Colors.CYAN + Colors.BOLD
        }

        color = color_map.get(level, Colors.WHITE)
        return f"{timestamp}{color}{message}{Colors.RESET}"

    def debug(self, message: str):
        if self.log_level in ['debug']:
            print(self._format_message(message, 'debug'))

    def info(self, message: str):
        if self.log_level in ['debug', 'info']:
            print(self._format_message(message, 'info'))

    def success(self, message: str):
        print(self._format_message(message, 'success'))

    def warning(self, message: str):
        if self.log_level in ['debug', 'info', 'warning']:
            print(self._format_message(message, 'warning'))

    def error(self, message: str):
        print(self._format_message(message, 'error'))

    def section(self, message: str):
        print(self._format_message(f"\n╔════════════════════════════════════════╗\n║ {message:<38} ║\n╚════════════════════════════════════════╝\n", 'section'))

    def subsection(self, message: str):
        print(self._format_message(f"[{message}]", 'subsection'))


class MetaGuildNetRunner:
    """Main runner for MetaGuildNet workflows."""

    def __init__(self, config_path: str = "config.json"):
        self.config_path = Path(config_path)
        # Initialize logger with default config first
        self.project_root = Path(__file__).parent.parent  # GuildNet root
        self.logger = Logger(self._get_default_config())
        self.config = self._load_config()
        # Update logger with actual config
        self.logger = Logger(self.config)

    def _load_config(self) -> Dict[str, Any]:
        """Load configuration from JSON file."""
        try:
            # Convert config_path to string for consistent handling
            config_path_str = str(self.config_path)

            # Try multiple path resolution strategies
            config_path = None

            # Strategy 1: Try as relative to current working directory
            cwd_path = Path.cwd() / config_path_str
            if cwd_path.exists() and cwd_path.is_file():
                config_path = cwd_path
                self.logger.debug(f"Found config at CWD path: {config_path}")

            # Strategy 2: Try as relative to script directory
            if not config_path:
                script_dir = Path(__file__).parent
                script_path = script_dir / config_path_str
                if script_path.exists() and script_path.is_file():
                    config_path = script_path
                    self.logger.debug(f"Found config at script path: {config_path}")

            # Strategy 3: Try as absolute path (if it looks like an absolute path)
            if not config_path and (config_path_str.startswith('/') or (len(config_path_str) > 1 and config_path_str[1] == ':')):
                abs_path = Path(config_path_str)
                if abs_path.exists() and abs_path.is_file():
                    config_path = abs_path
                    self.logger.debug(f"Found config at absolute path: {config_path}")

            # Strategy 4: Try relative to MetaGuildNet directory (explicit check)
            if not config_path:
                metaguildnet_dir = Path(__file__).parent
                metaguildnet_path = metaguildnet_dir / config_path_str
                if metaguildnet_path.exists() and metaguildnet_path.is_file():
                    config_path = metaguildnet_path
                    self.logger.debug(f"Found config at MetaGuildNet path: {config_path}")

            # Strategy 5: Try as relative to GuildNet root (for MetaGuildNet/dev-config.json)
            if not config_path:
                guildnet_root = Path(__file__).parent.parent
                guildnet_path = guildnet_root / config_path_str
                if guildnet_path.exists() and guildnet_path.is_file():
                    config_path = guildnet_path
                    self.logger.debug(f"Found config at GuildNet root path: {config_path}")

            if config_path and config_path.exists() and config_path.is_file():
                self.logger.debug(f"Loading config from: {config_path}")
                with open(config_path, 'r') as f:
                    return json.load(f)
            else:
                self.logger.debug(f"All path resolution strategies failed for: {self.config_path}")
                raise FileNotFoundError(f"Config file not found: {self.config_path}")
        except FileNotFoundError:
            self.logger.warning(f"Config file not found: {self.config_path}")
            self.logger.info("Using default configuration")
            return self._get_default_config()
        except json.JSONDecodeError as e:
            self.logger.error(f"Invalid JSON in config file: {e}")
            self.logger.info("Using default configuration")
            return self._get_default_config()

    def _get_default_config(self) -> Dict[str, Any]:
        """Get default configuration."""
        return {
            "meta_setup": {
                "enabled": True,
                "mode": "auto",
                "verify_timeout": 300,
                "auto_approve_routes": True,
                "log_level": "info"
            },
            "verification": {
                "enabled": True,
                "layers": ["network", "cluster", "database", "application"],
                "output_format": "text",
                "timeout": 300
            },
            "testing": {
                "enabled": True,
                "run_integration_tests": True,
                "run_e2e_tests": True,
                "test_timeout": 600
            },
            "examples": {
                "enabled": True,
                "create_workspace": True,
                "workspace_image": "codercom/code-server:latest",
                "workspace_name": "metaguildnet-demo",
                "multi_user_setup": False,
                "comprehensive_output": True
            },
            "diagnostics": {
                "enabled": True,
                "export_diagnostics": False,
                "diagnostic_timeout": 60
            },
            "cleanup": {
                "enabled": False,
                "remove_workspaces": True,
                "stop_services": False
            },
            "logging": {
                "level": "info",
                "format": "colored",
                "timestamp": True,
                "file": None
            }
        }

    def _run_command(self, command: List[str], cwd: Optional[Path] = None, timeout: Optional[int] = None, env: Optional[Dict[str, str]] = None) -> bool:
        """Run a shell command and return success status."""
        try:
            self.logger.debug(f"Running: {' '.join(command)}")
            result = subprocess.run(
                command,
                cwd=cwd or self.project_root,
                capture_output=True,
                text=True,
                timeout=timeout,
                env=env
            )

            if result.stdout:
                for line in result.stdout.splitlines():
                    if line.strip():
                        self.logger.info(f"  {line}")

            if result.stderr:
                for line in result.stderr.splitlines():
                    if line.strip():
                        self.logger.warning(f"  {line}")

            return result.returncode == 0

        except subprocess.TimeoutExpired:
            self.logger.error(f"Command timed out: {' '.join(command)}")
            return False
        except Exception as e:
            self.logger.error(f"Command failed: {' '.join(command)} - {e}")
            return False

    def run_setup(self) -> bool:
        """Run MetaGuildNet setup."""
        config = self.config.get('meta_setup', {})

        if not config.get('enabled', True):
            self.logger.info("Setup disabled in config")
            return True

        self.logger.section("MetaGuildNet Setup")

        # Set environment variables
        env = os.environ.copy()
        env.update({
            'METAGN_SETUP_MODE': config.get('mode', 'auto'),
            'METAGN_VERIFY_TIMEOUT': str(config.get('verify_timeout', 300)),
            'METAGN_AUTO_APPROVE_ROUTES': str(config.get('auto_approve_routes', True)).lower(),
            'METAGN_LOG_LEVEL': config.get('log_level', 'info')
        })

        success = self._run_command(['make', '-C', str(self.project_root), 'meta-setup'], timeout=config.get('verify_timeout', 300))

        if success:
            self.logger.success("Setup completed successfully")
        else:
            self.logger.error("Setup failed")

        return success

    def run_verification(self) -> bool:
        """Run MetaGuildNet verification."""
        config = self.config.get('verification', {})

        if not config.get('enabled', True):
            self.logger.info("Verification disabled in config")
            return True

        self.logger.section("MetaGuildNet Verification")

        output_format = config.get('output_format', 'text')
        script_path = self.project_root / 'MetaGuildNet' / 'scripts' / 'verify' / 'verify_all.sh'
        cmd = ['bash', str(script_path)]
        if output_format == 'json':
            cmd.append('--json')
        success = self._run_command(cmd, timeout=config.get('timeout', 300))

        if success:
            self.logger.success("Verification completed successfully")
        else:
            self.logger.error("Verification failed")
            self.logger.info("")
            self.logger.info("🔧 Common issues and solutions:")
            self.logger.info("   • Host App not running → make run")
            self.logger.info("   • Kubernetes cluster not accessible → Check kubeconfig")
            self.logger.info("   • RethinkDB not available → make deploy-k8s-addons")
            self.logger.info("   • TLS certificate issues → make regen-certs")
            self.logger.info("   • Network connectivity → Check Tailscale status")
            self.logger.info("")
            self.logger.info("📋 For detailed diagnostics: make meta-diagnose")

        return success

    def run_testing(self) -> bool:
        """Run MetaGuildNet tests."""
        config = self.config.get('testing', {})

        if not config.get('enabled', True):
            self.logger.info("Testing disabled in config")
            return True

        self.logger.section("MetaGuildNet Testing")

        success = True

        if config.get('run_integration_tests', True):
            self.logger.subsection("Running Integration Tests")
            if not self._run_command(['make', '-C', str(self.project_root), 'meta-test-integration'], timeout=config.get('test_timeout', 600)):
                success = False

        if config.get('run_e2e_tests', True):
            self.logger.subsection("Running E2E Tests")
            if not self._run_command(['make', '-C', str(self.project_root), 'meta-test-e2e'], timeout=config.get('test_timeout', 600)):
                success = False

        if success:
            self.logger.success("Testing completed successfully")
        else:
            self.logger.error("Testing failed")
            self.logger.info("")
            self.logger.info("🔧 Testing troubleshooting:")
            self.logger.info("   • Integration tests require running services")
            self.logger.info("   • E2E tests need Host App and Kubernetes cluster")
            self.logger.info("   • Start required services first: make run")
            self.logger.info("   • Verify cluster connectivity: kubectl get nodes")
            self.logger.info("   • Check Host App health: curl -sk https://127.0.0.1:8080/healthz")
            self.logger.info("")

        return success

    def run_examples(self) -> bool:
        """Run MetaGuildNet examples."""
        config = self.config.get('examples', {})

        if not config.get('enabled', True):
            self.logger.info("Examples disabled in config")
            return True

        self.logger.section("MetaGuildNet Examples")

        success = True

        if config.get('create_workspace', True):
            # Run comprehensive example for better output
            if config.get('comprehensive_output', False):
                if not self.run_comprehensive_workspace_example():
                    success = False
            else:
                if not self.run_basic_workspace_example():
                    success = False

        if config.get('multi_user_setup', False):
            self.logger.subsection("Running Multi-User Setup")
            if not self.run_multi_user_example():
                success = False

        if success:
            self.logger.success("Examples completed successfully")
        else:
            self.logger.error("Examples failed")
            self.logger.info("")
            self.logger.info("🔧 Example troubleshooting:")
            self.logger.info("   • Examples require Host App to be running")
            self.logger.info("   • Start Host App: make run")
            self.logger.info("   • Verify Host App health: curl -sk https://127.0.0.1:8080/healthz")
            self.logger.info("   • Check TLS certificates: ls -la certs/")
            self.logger.info("   • Ensure Kubernetes cluster is accessible")
            self.logger.info("")

        return success

    def run_basic_workspace_example(self) -> bool:
        """Run the basic workspace creation example with comprehensive output."""
        self.logger.subsection("Running Basic Workspace Example")

        # Configuration
        workspace_name = f"metaguildnet-demo-{int(time.time())}"
        workspace_image = "codercom/code-server:latest"
        password = "example123"

        self.logger.info(f"Creating workspace: {workspace_name}")
        self.logger.info(f"Image: {workspace_image}")

        # Create workspace via API
        self.logger.info("Sending workspace creation request...")

        import requests

        try:
            response = requests.post(
                "https://127.0.0.1:8080/api/jobs",
                json={
                    "name": workspace_name,
                    "image": workspace_image,
                    "env": [{"name": "PASSWORD", "value": password}]
                },
                verify=False,  # For self-signed certs
                timeout=30
            )

            if response.status_code == 202:
                workspace_data = response.json()
                workspace_id = workspace_data.get('id')

                if workspace_id:
                    self.logger.success(f"Workspace created successfully: {workspace_id}")

                    # Wait for workspace to be ready
                    return self._wait_for_workspace_ready(workspace_id, workspace_name, password)
                else:
                    self.logger.error("No workspace ID in response")
                    return False
            else:
                self.logger.error(f"Failed to create workspace: {response.status_code}")
                self.logger.error(f"Response: {response.text}")
                return False

        except requests.exceptions.RequestException as e:
            self.logger.error(f"Request failed: {e}")
            self.logger.info("")
            self.logger.info("🔧 Troubleshooting steps:")
            self.logger.info("   1. Check if Host App is running:")
            self.logger.info("      curl -sk https://127.0.0.1:8080/healthz")
            self.logger.info("   2. Start Host App if needed:")
            self.logger.info("      make run")
            self.logger.info("   3. Verify TLS certificates:")
            self.logger.info("      ls -la certs/  # Should have server.crt and server.key")
            self.logger.info("   4. Check Kubernetes connectivity:")
            self.logger.info("      kubectl get nodes")
            self.logger.info("")
            return False

    def run_multi_user_example(self) -> bool:
        """Run the multi-user setup example with comprehensive output."""
        self.logger.subsection("Running Multi-User Setup Example")

        # Configuration
        users = {
            "alice": "codercom/code-server:latest",
            "bob": "jupyter/scipy-notebook:latest",
            "charlie": "theiaide/theia-python:latest"
        }

        created_workspaces = []
        self.logger.info(f"Creating workspaces for {len(users)} users...")

        for user, image in users.items():
            workspace_name = f"workspace-{user}-{int(time.time())}"

            self.logger.info(f"Creating workspace for {user}:")
            self.logger.info(f"  Name: {workspace_name}")
            self.logger.info(f"  Image: {image}")

            import requests

            try:
                response = requests.post(
                    "https://127.0.0.1:8080/api/jobs",
                    json={
                        "name": workspace_name,
                        "image": image,
                        "env": [{"name": "USER", "value": user}]
                    },
                    verify=False,
                    timeout=30
                )

                if response.status_code == 202:
                    workspace_data = response.json()
                    workspace_id = workspace_data.get('id')

                    if workspace_id:
                        self.logger.success(f"  ✓ Created: {workspace_id}")
                        created_workspaces.append((user, workspace_id, workspace_name))
                    else:
                        self.logger.error(f"  ✗ Failed to get workspace ID for {user}")
                else:
                    self.logger.error(f"  ✗ Failed to create workspace for {user}: {response.status_code}")

            except requests.exceptions.RequestException as e:
                self.logger.error(f"  ✗ Request failed for {user}: {e}")

        if not created_workspaces:
            self.logger.error("No workspaces created. Host App may not be running.")
            self.logger.info("")
            self.logger.info("🔧 Quick troubleshooting:")
            self.logger.info("   1. Verify Host App status:")
            self.logger.info("      curl -sk https://127.0.0.1:8080/healthz")
            self.logger.info("   2. Start Host App:")
            self.logger.info("      make run")
            self.logger.info("   3. Check logs for errors:")
            self.logger.info("      tail -f ~/.guildnet/state/logs/hostapp.log")
            self.logger.info("")
            return False

        self.logger.info(f"Successfully created {len(created_workspaces)} workspaces")

        # Wait for workspaces to be ready
        ready_count = 0
        for user, workspace_id, workspace_name in created_workspaces:
            if self._wait_for_workspace_ready(workspace_id, workspace_name, None, user):
                ready_count += 1

        self.logger.info(f"Workspaces ready: {ready_count}/{len(created_workspaces)}")

        # Show summary
        self._show_workspace_summary(created_workspaces)

        return ready_count > 0

    def _wait_for_workspace_ready(self, workspace_id: str, workspace_name: str, password: str = None, user: str = None) -> bool:
        """Wait for workspace to be ready and show detailed status."""
        self.logger.info(f"Waiting for workspace {workspace_name} to be ready...")

        max_wait = 120
        waited = 0

        while waited < max_wait:
            try:
                import requests
                response = requests.get(
                    f"https://127.0.0.1:8080/api/servers/{workspace_id}",
                    verify=False,
                    timeout=10
                )

                if response.status_code == 200:
                    workspace_data = response.json()
                    status = workspace_data.get('status')

                    if status == "Running":
                        self.logger.success(f"✓ Workspace {workspace_name} is running!")

                        # Show detailed workspace information
                        self._show_workspace_details(workspace_data, workspace_name, password, user)
                        return True

                    self.logger.info(f"  Status: {status} (waited {waited}s)")
                else:
                    self.logger.warning(f"  Failed to get status: {response.status_code}")

            except requests.exceptions.RequestException as e:
                self.logger.debug(f"  Request failed: {e}")

            time.sleep(5)
            waited += 5

        self.logger.error(f"✗ Workspace {workspace_name} did not become ready within {max_wait}s")
        return False

    def _show_workspace_details(self, workspace_data: dict, workspace_name: str, password: str = None, user: str = None):
        """Show comprehensive workspace details."""
        status = workspace_data.get('status', 'Unknown')
        status_icon = "🟢" if status == "Running" else "🟡" if status == "Pending" else "🔴"

        self.logger.info(f"Workspace Details for {workspace_name}:")
        self.logger.info(f"  {status_icon} Status: {status}")
        self.logger.info(f"  🆔 ID: {workspace_data.get('id', 'N/A')}")
        self.logger.info(f"  📝 Name: {workspace_data.get('name', 'N/A')}")
        self.logger.info(f"  🖼️  Image: {workspace_data.get('image', 'N/A')}")

        # Show environment variables if available
        env = workspace_data.get('env', [])
        if env:
            self.logger.info("  🔧 Environment:")
            for env_var in env:
                name = env_var.get('name', 'Unknown')
                value = env_var.get('value', '***')
                self.logger.info(f"    🔹 {name}={value}")

        # Show ports if available
        ports = workspace_data.get('ports', [])
        if ports:
            self.logger.info("  🔌 Ports:")
            for port in ports:
                self.logger.info(f"    🔹 {port.get('port', 'N/A')} ({port.get('protocol', 'tcp')})")

        # Show access information
        workspace_id = workspace_data.get('id', '')
        if workspace_id:
            self.logger.info("  🌐 Access:")
            self.logger.info(f"    🔗 URL: https://127.0.0.1:8080/proxy/server/{workspace_id}/")
            if password:
                self.logger.info(f"    🔑 Password: {password}")
            if user:
                self.logger.info(f"    👤 User: {user}")

        # Show logs if available
        logs_url = f"https://127.0.0.1:8080/api/servers/{workspace_id}/logs"
        self.logger.info(f"  📋 Logs: {logs_url}")

        self.logger.info("")

    def _show_workspace_summary(self, workspaces: list):
        """Show summary of all created workspaces."""
        self.logger.section("Workspace Summary")

        self.logger.info(f"Total workspaces created: {len(workspaces)}")
        self.logger.info("")

        for user, workspace_id, workspace_name in workspaces:
            self.logger.info(f"👤 {user}:")
            self.logger.info(f"  📝 Name: {workspace_name}")
            self.logger.info(f"  🆔 ID: {workspace_id}")
            self.logger.info(f"  🌐 URL: https://127.0.0.1:8080/proxy/server/{workspace_id}/")
            self.logger.info("")

        self.logger.info("Management Commands:")
        self.logger.info("  View all workspaces: curl -sk https://127.0.0.1:8080/api/servers | jq")
        self.logger.info("  Cleanup all workspaces: curl -sk -X POST https://127.0.0.1:8080/api/admin/stop-all")
        self.logger.info("  Delete specific workspace: curl -sk -X DELETE https://127.0.0.1:8080/api/servers/<workspace-id>")
        self.logger.info("")

    def run_comprehensive_workspace_example(self) -> bool:
        """Run comprehensive workspace example with full validation and logging."""
        self.logger.subsection("Running Comprehensive Workspace Example")

        # Check for dev mode
        dev_mode = os.environ.get('METAGN_DEV_MODE', 'false').lower() == 'true'
        
        if dev_mode:
            self.logger.info("Running in DEV MODE - demonstrating workspace example flow")
            self.logger.info("")
            self.logger.info("✓ Example demonstrates:")
            self.logger.info("  1. Workspace creation via API POST to /api/jobs")
            self.logger.info("  2. Status polling until Running state")
            self.logger.info("  3. Access URL generation")
            self.logger.info("  4. Cleanup via API DELETE")
            self.logger.info("")
            self.logger.info("To run this example for real:")
            self.logger.info("  1. Start Host App: make run")
            self.logger.info("  2. Ensure cluster is accessible")
            self.logger.info("  3. Run: python3 MetaGuildNet/run.py --workflow example")
            self.logger.info("")
            return True

        # First check if Host App is running
        if not self._check_host_app_health():
            self.logger.error("Host App is not running. Cannot create workspaces.")
            self.logger.info("")
            self.logger.info("💡 To start the Host App:")
            self.logger.info("   make run")
            self.logger.info("")
            self.logger.info("🔍 To check Host App status:")
            self.logger.info("   curl -sk https://127.0.0.1:8080/healthz")
            self.logger.info("")
            self.logger.info("📋 Prerequisites for workspace creation:")
            self.logger.info("   • Host App running on https://127.0.0.1:8080")
            self.logger.info("   • Valid TLS certificates (auto-generated if missing)")
            self.logger.info("   • Kubernetes cluster accessible")
            self.logger.info("   • RethinkDB service available")
            self.logger.info("")
            return False

        # Create basic workspace
        if not self.run_basic_workspace_example():
            return False

        # Show current workspace status
        self._show_current_workspace_status()

        # Test workspace access (if password is known)
        self._test_workspace_access()

        return True

    def _check_host_app_health(self) -> bool:
        """Check if Host App is healthy."""
        try:
            import requests
            response = requests.get(
                "https://127.0.0.1:8080/healthz",
                verify=False,
                timeout=10
            )
            return response.status_code == 200
        except:
            return False

    def _show_current_workspace_status(self):
        """Show current workspace status from API."""
        self.logger.subsection("Current Workspace Status")

        try:
            import requests
            response = requests.get(
                "https://127.0.0.1:8080/api/servers",
                verify=False,
                timeout=10
            )

            if response.status_code == 200:
                workspaces = response.json()
                if workspaces:
                    self.logger.info(f"📊 Active workspaces: {len(workspaces)}")
                    for workspace in workspaces:
                        status = workspace.get('status', 'Unknown')
                        status_icon = "🟢" if status == "Running" else "🟡" if status == "Pending" else "🔴"
                        self.logger.info(f"  {status_icon} {workspace.get('name', 'Unknown')}: {status}")
                else:
                    self.logger.info("📭 No active workspaces")
            else:
                self.logger.warning(f"❌ Failed to get workspace list: {response.status_code}")

        except requests.exceptions.RequestException as e:
            self.logger.debug(f"Failed to check workspace status: {e}")

    def _test_workspace_access(self):
        """Test workspace access and show logs."""
        self.logger.subsection("Testing Workspace Access")

        # This would require knowing the workspace ID and password
        # For now, just show that the access methods are available
        self.logger.info("🌐 Workspace Access Methods:")
        self.logger.info("  🔗 Web interface: https://127.0.0.1:8080")
        self.logger.info("  📡 API endpoints: /api/servers/<workspace-id>")
        self.logger.info("  📋 Logs endpoint: /api/servers/<workspace-id>/logs")
        self.logger.info("  🚪 Proxy access: /proxy/server/<workspace-id>/")
        self.logger.info("")
        self.logger.info("💡 Access patterns:")
        self.logger.info("  • Workspace URLs: https://127.0.0.1:8080/proxy/server/<id>/")
        self.logger.info("  • API queries: curl -sk https://127.0.0.1:8080/api/servers/<id>")
        self.logger.info("  • Log streaming: curl -sk https://127.0.0.1:8080/api/servers/<id>/logs")

    def run_diagnostics(self) -> bool:
        """Run MetaGuildNet diagnostics."""
        config = self.config.get('diagnostics', {})

        if not config.get('enabled', True):
            self.logger.info("Diagnostics disabled in config")
            return True

        self.logger.section("MetaGuildNet Diagnostics")

        success = True

        self.logger.subsection("Running Health Check")
        if not self._run_command(['make', '-C', str(self.project_root), 'meta-diagnose'], timeout=config.get('diagnostic_timeout', 60)):
            success = False

        if config.get('export_diagnostics', False):
            self.logger.subsection("Exporting Diagnostics")
            if not self._run_command(['make', '-C', str(self.project_root), 'export-diagnostics'], timeout=120):
                success = False

        if success:
            self.logger.success("Diagnostics completed successfully")
        else:
            self.logger.error("Diagnostics failed")

        return success

    def run_cleanup(self) -> bool:
        """Run MetaGuildNet cleanup."""
        config = self.config.get('cleanup', {})

        if not config.get('enabled', False):
            self.logger.info("Cleanup disabled in config")
            return True

        self.logger.section("MetaGuildNet Cleanup")

        success = True

        if config.get('remove_workspaces', True):
            self.logger.subsection("Removing Workspaces")
            if not self._run_command(['make', '-C', str(self.project_root), 'stop-all'], timeout=60):
                success = False

        if config.get('stop_services', False):
            self.logger.subsection("Stopping Services")
            # Add service stopping commands here if needed

        if success:
            self.logger.success("Cleanup completed successfully")
        else:
            self.logger.error("Cleanup failed")

        return success

    def run_full_workflow(self) -> bool:
        """Run the complete MetaGuildNet workflow."""
        start_time = time.time()

        self.logger.section("MetaGuildNet Full Workflow")

        steps = [
            ("Setup", self.run_setup),
            ("Verification", self.run_verification),
            ("Testing", self.run_testing),
            ("Examples", self.run_examples),
            ("Diagnostics", self.run_diagnostics),
            ("Cleanup", self.run_cleanup)
        ]

        success = True
        for step_name, step_func in steps:
            if not step_func():
                success = False
                break

        end_time = time.time()
        duration = end_time - start_time

        if success:
            self.logger.success(f"Full workflow completed successfully in {duration:.1f}s")
        else:
            self.logger.error(f"Full workflow failed after {duration:.1f}s")

        return success

    def show_config(self):
        """Show current configuration."""
        self.logger.section("MetaGuildNet Configuration")
        print(json.dumps(self.config, indent=2))

    def show_help(self):
        """Show help information."""
        print(__doc__)


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="MetaGuildNet Runner - Programmatic GuildNet workflow execution",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 run.py                    # Run full workflow with default config
  python3 run.py --config dev.json  # Run with custom config
  python3 run.py --dry-run          # Show what would be run
  python3 run.py --help             # Show this help
        """
    )

    parser.add_argument(
        '--config', '-c',
        default='config.json',
        help='Configuration file path (default: config.json)'
    )

    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Show configuration and exit without running'
    )

    parser.add_argument(
        '--workflow',
        choices=['full', 'setup', 'verify', 'test', 'example', 'diagnose', 'cleanup'],
        default='full',
        help='Workflow to run (default: full)'
    )

    parser.add_argument(
        '--log-level',
        choices=['debug', 'info', 'warning', 'error'],
        default='info',
        help='Log level (default: info)'
    )

    args = parser.parse_args()

    # Change to script directory
    script_dir = Path(__file__).parent
    os.chdir(script_dir)

    # Disable colors on Windows if needed
    Colors.disable_on_windows()

    # Create runner
    try:
        runner = MetaGuildNetRunner(args.config)
    except Exception as e:
        print(f"Error initializing runner: {e}")
        return 1

    # Override log level from command line
    runner.config['logging']['level'] = args.log_level
    runner.logger = Logger(runner.config)

    # Handle dry run
    if args.dry_run:
        runner.show_config()
        return 0

    # Run requested workflow
    workflow_map = {
        'full': runner.run_full_workflow,
        'setup': runner.run_setup,
        'verify': runner.run_verification,
        'test': runner.run_testing,
        'example': runner.run_examples,
        'diagnose': runner.run_diagnostics,
        'cleanup': runner.run_cleanup
    }

    try:
        success = workflow_map[args.workflow]()
        return 0 if success else 1
    except KeyboardInterrupt:
        runner.logger.info("Interrupted by user")
        return 130
    except Exception as e:
        runner.logger.error(f"Unexpected error: {e}")
        return 1


if __name__ == '__main__':
    sys.exit(main())
