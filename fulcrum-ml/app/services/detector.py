"""insightface-backed face detector.

Heavy dependencies (insightface, onnxruntime, cv2, numpy) are imported lazily
inside `load`/`detect` so the FastAPI app — and its tests — can import this
module without the ML stack installed.
"""

from __future__ import annotations

from dataclasses import dataclass

from loguru import logger

from app.core.config import Settings


class DetectorError(Exception):
    """Raised when an image cannot be decoded or the model is unavailable."""


@dataclass
class DetectedFace:
    bbox: list[float]
    det_score: float
    embedding: list[float]


class Detector:
    """Wraps an insightface FaceAnalysis app. Not thread-safe for `load`."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._app = None  # insightface.app.FaceAnalysis once loaded

    @property
    def ready(self) -> bool:
        return self._app is not None

    def load(self) -> None:
        """Download (if needed) and initialize the model. Blocking."""
        if self._app is not None:
            return
        import insightface  # deferred: pulls onnxruntime

        s = self._settings
        logger.info("loading insightface model {} (root={})", s.model_name, s.model_root)
        app = insightface.app.FaceAnalysis(name=s.model_name, root=s.model_root)
        app.prepare(ctx_id=s.ctx_id, det_size=(s.det_size, s.det_size), det_thresh=s.det_score)
        self._app = app
        logger.info("model ready")

    def detect(self, image_bytes: bytes) -> list[DetectedFace]:
        if self._app is None:
            raise DetectorError("model not loaded")

        import cv2
        import numpy as np

        arr = np.frombuffer(image_bytes, dtype=np.uint8)
        img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
        if img is None:
            raise DetectorError("could not decode image")

        faces = self._app.get(img)
        results: list[DetectedFace] = []
        for f in faces:
            # normed_embedding is already L2-normalized, so the Go matcher can
            # use a plain dot product as cosine similarity.
            results.append(
                DetectedFace(
                    bbox=[float(x) for x in f.bbox],
                    det_score=float(f.det_score),
                    embedding=[float(x) for x in f.normed_embedding],
                )
            )
        return results
