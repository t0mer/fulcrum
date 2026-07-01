"""Settings for fulcrum-ml, sourced from environment variables.

All variables use the FULCRUM_ML_ prefix (e.g. FULCRUM_ML_MODEL_NAME).
"""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    # protected_namespaces=() lets us use model_name/model_root without the
    # pydantic "model_" namespace warning.
    model_config = SettingsConfigDict(
        env_prefix="FULCRUM_ML_", extra="ignore", protected_namespaces=()
    )

    # insightface model pack. buffalo_l = default 512-d ArcFace + SCRFD detector.
    model_name: str = "buffalo_l"
    # Where insightface downloads/caches weights. Mount this as a volume so the
    # non-redistributable pretrained models are fetched once at runtime.
    model_root: str = "/models"

    # Detection input size (square). Larger = more accurate, slower on CPU.
    det_size: int = 640
    # Minimum detection score to keep a face. Do not go below 0.5 (see contract).
    det_score: float = 0.5

    # onnxruntime execution: -1 = CPU. Set >= 0 to use a GPU device id.
    ctx_id: int = -1

    log_level: str = "INFO"
    host: str = "0.0.0.0"
    port: int = 8081


settings = Settings()
