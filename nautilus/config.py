import os
import tomllib
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class RiskConfig:
    max_position_size_pct: float = 2.0
    daily_loss_limit_pct: float = 3.0
    weekly_loss_limit_pct: float = 5.0
    max_drawdown_pct: float = 10.0
    kill_switch_enabled: bool = True


@dataclass
class NautilusConfig:
    enabled: bool = True
    python_path: str = "python3"
    grpc_port: int = 50051
    grpc_address: str = "localhost"
    auto_start: bool = True
    data_catalog: str = "~/.tradefox/data"
    strategies_dir: str = "~/.tradefox/strategies"
    risk: RiskConfig = field(default_factory=RiskConfig)


def load_config() -> NautilusConfig:
    """Load NautilusConfig from ~/.tradefox/config.toml."""
    config_path = Path.home() / ".tradefox" / "config.toml"
    if not config_path.exists():
        return NautilusConfig()

    with open(config_path, "rb") as f:
        data = tomllib.load(f)

    nautilus_data = data.get("nautilus", {})
    risk_data = nautilus_data.pop("risk", {})

    risk = RiskConfig(
        max_position_size_pct=risk_data.get("max_position_size_pct", 2.0),
        daily_loss_limit_pct=risk_data.get("daily_loss_limit_pct", 3.0),
        weekly_loss_limit_pct=risk_data.get("weekly_loss_limit_pct", 5.0),
        max_drawdown_pct=risk_data.get("max_drawdown_pct", 10.0),
        kill_switch_enabled=risk_data.get("kill_switch_enabled", True),
    )

    return NautilusConfig(
        enabled=nautilus_data.get("enabled", True),
        python_path=nautilus_data.get("python_path", "python3"),
        grpc_port=nautilus_data.get("grpc_port", 50051),
        grpc_address=nautilus_data.get("grpc_address", "localhost"),
        auto_start=nautilus_data.get("auto_start", True),
        data_catalog=nautilus_data.get("data_catalog", "~/.tradefox/data"),
        strategies_dir=nautilus_data.get("strategies_dir", "~/.tradefox/strategies"),
        risk=risk,
    )
