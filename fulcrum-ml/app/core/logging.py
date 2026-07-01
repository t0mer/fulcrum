"""Loguru configuration for fulcrum-ml."""

from __future__ import annotations

import sys

from loguru import logger

from app.core.config import settings


def configure_logging() -> None:
    logger.remove()
    logger.add(
        sys.stderr,
        level=settings.log_level.upper(),
        backtrace=False,
        diagnose=False,
    )
