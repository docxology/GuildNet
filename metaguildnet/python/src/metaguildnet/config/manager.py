"""Configuration management for MetaGuildNet"""

import os
from pathlib import Path
from typing import Any, Dict

import yaml


class ConfigManager:
    """Manage MetaGuildNet configuration"""

    def __init__(self, config_path: Path):
        self.config_path = config_path

    def load(self) -> Dict[str, Any]:
        """Load configuration from file and environment"""
        config = self._load_defaults()

        # Load from file if exists
        if self.config_path.exists():
            with open(self.config_path) as f:
                file_config = yaml.safe_load(f) or {}
                config = self._merge(config, file_config)

        # Override with environment variables
        config = self._apply_env_overrides(config)

        return config

    def save(self, config: Dict[str, Any]) -> None:
        """Save configuration to file"""
        self.config_path.parent.mkdir(parents=True, exist_ok=True)

        with open(self.config_path, "w") as f:
            yaml.dump(config, f, default_flow_style=False)

    def _load_defaults(self) -> Dict[str, Any]:
        """Load default configuration"""
        return {
            "api": {
                "base_url": "https://localhost:8090",
                "token": "",
                "timeout": 30,
            },
            "defaults": {
                "cluster": "",
                "format": "table",
            },
            "logging": {
                "level": "info",
            },
        }

    def _merge(self, base: Dict[str, Any], override: Dict[str, Any]) -> Dict[str, Any]:
        """Merge two configuration dictionaries"""
        result = base.copy()

        for key, value in override.items():
            if key in result and isinstance(result[key], dict) and isinstance(value, dict):
                result[key] = self._merge(result[key], value)
            else:
                result[key] = value

        return result

    def _apply_env_overrides(self, config: Dict[str, Any]) -> Dict[str, Any]:
        """Apply environment variable overrides"""
        if url := os.getenv("MGN_API_URL"):
            config["api"]["base_url"] = url

        if token := os.getenv("MGN_API_TOKEN"):
            config["api"]["token"] = token

        if cluster := os.getenv("MGN_DEFAULT_CLUSTER"):
            config["defaults"]["cluster"] = cluster

        return config

