# Stage 1: build wheels. insightface ships a Cython C++ extension that must be
# compiled, so this stage needs a toolchain; the runtime image stays slim.
FROM python:3.12-slim AS builder

ENV PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt .
# Install into a relocatable prefix that we copy into the runtime image.
RUN pip install --prefix=/install -r requirements.txt

# Stage 2: slim runtime — no build tools.
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    FULCRUM_ML_MODEL_ROOT=/models

WORKDIR /app

# Runtime libs: libgomp1 for onnxruntime, libglib2.0-0 for opencv-headless.
RUN apt-get update && apt-get install -y --no-install-recommends \
      libgomp1 \
      libglib2.0-0 \
      curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /install /usr/local
COPY app/ /app/app/

# Model cache (mounted volume). insightface downloads non-redistributable
# weights here at runtime; they are never baked into the image.
RUN mkdir -p /models \
    && useradd --create-home --uid 10001 appuser \
    && chown -R appuser:appuser /app /models
USER appuser

EXPOSE 8081
VOLUME ["/models"]

# /healthz is up as soon as the process is; readiness (/readyz) gates on the model.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8081/healthz || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8081"]
