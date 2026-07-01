"""API tests using a fake detector so the ML stack isn't required to run them."""

from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from app.main import app, get_detector
from app.services.detector import DetectedFace, DetectorError


class FakeDetector:
    def __init__(self, ready: bool = True, faces=None, error: Exception | None = None):
        self._ready = ready
        self._faces = faces or []
        self._error = error

    @property
    def ready(self) -> bool:
        return self._ready

    def detect(self, image_bytes: bytes):
        if self._error is not None:
            raise self._error
        return self._faces


@pytest.fixture
def client():
    with TestClient(app) as c:
        yield c
    app.dependency_overrides.clear()


def test_healthz_always_ok(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector(ready=False)
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_readyz_503_until_loaded(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector(ready=False)
    assert client.get("/readyz").status_code == 503


def test_readyz_ok_when_loaded(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector(ready=True)
    resp = client.get("/readyz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ready"


def test_detect_returns_faces(client):
    face = DetectedFace(bbox=[1.0, 2.0, 3.0, 4.0], det_score=0.99, embedding=[0.1] * 512)
    app.dependency_overrides[get_detector] = lambda: FakeDetector(faces=[face])
    resp = client.post("/detect", files={"file": ("x.jpg", b"not-a-real-image", "image/jpeg")})
    assert resp.status_code == 200
    body = resp.json()
    assert len(body["faces"]) == 1
    assert body["faces"][0]["det_score"] == 0.99
    assert len(body["faces"][0]["embedding"]) == 512


def test_detect_503_when_not_ready(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector(ready=False)
    resp = client.post("/detect", files={"file": ("x.jpg", b"data", "image/jpeg")})
    assert resp.status_code == 503


def test_detect_400_on_empty_file(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector()
    resp = client.post("/detect", files={"file": ("x.jpg", b"", "image/jpeg")})
    assert resp.status_code == 400


def test_detect_400_on_undecodable_image(client):
    app.dependency_overrides[get_detector] = lambda: FakeDetector(
        error=DetectorError("could not decode image")
    )
    resp = client.post("/detect", files={"file": ("x.jpg", b"bad", "image/jpeg")})
    assert resp.status_code == 400
