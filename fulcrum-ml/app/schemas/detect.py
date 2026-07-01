"""Request/response schemas for the /detect endpoint."""

from __future__ import annotations

from pydantic import BaseModel, Field


class Face(BaseModel):
    bbox: list[float] = Field(..., description="[x1, y1, x2, y2] pixel coordinates")
    det_score: float = Field(..., description="Detector confidence for this face")
    embedding: list[float] = Field(..., description="512-d L2-normalized ArcFace embedding")


class DetectResponse(BaseModel):
    faces: list[Face]


class HealthResponse(BaseModel):
    status: str
    version: str
