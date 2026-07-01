"""fulcrum-ml FastAPI application.

Contract (see CLAUDE.md §8):
  POST /detect  multipart file=<image> -> {"faces": [{bbox, det_score, embedding}]}
  GET  /healthz -> 200 while the process is up
  GET  /readyz  -> 200 only once the model has loaded

No authentication: this service is bound to the compose internal network only.
"""

from __future__ import annotations

import asyncio
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, File, HTTPException, UploadFile
from loguru import logger

from app import __version__
from app.core.config import settings
from app.core.logging import configure_logging
from app.schemas.detect import DetectResponse, Face, HealthResponse
from app.services.detector import Detector, DetectorError

configure_logging()


def get_detector() -> Detector:
    """Dependency accessor; overridden in tests."""
    return _detector


_detector = Detector(settings)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Load the model off the event loop so /healthz answers immediately while
    # weights download. /readyz stays 503 until this finishes.
    async def _load() -> None:
        try:
            await asyncio.to_thread(_detector.load)
        except Exception:  # noqa: BLE001 - log and stay not-ready
            logger.exception("model load failed")

    task = asyncio.create_task(_load())
    yield
    task.cancel()


app = FastAPI(
    title="fulcrum-ml",
    version=__version__,
    docs_url="/api/docs",
    redoc_url=None,
    lifespan=lifespan,
)


@app.get("/healthz", response_model=HealthResponse)
async def healthz() -> HealthResponse:
    return HealthResponse(status="ok", version=__version__)


@app.get("/readyz")
async def readyz(detector: Detector = Depends(get_detector)):
    if not detector.ready:
        raise HTTPException(status_code=503, detail="model not loaded")
    return {"status": "ready"}


@app.post("/detect", response_model=DetectResponse)
async def detect(
    file: UploadFile = File(...),
    detector: Detector = Depends(get_detector),
) -> DetectResponse:
    if not detector.ready:
        raise HTTPException(status_code=503, detail="model not loaded")
    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty file")
    try:
        faces = await asyncio.to_thread(detector.detect, data)
    except DetectorError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return DetectResponse(
        faces=[Face(bbox=f.bbox, det_score=f.det_score, embedding=f.embedding) for f in faces]
    )
