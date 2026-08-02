FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

COPY pyproject.toml ./
COPY worker ./worker

RUN pip install --no-cache-dir .

EXPOSE 8082

CMD ["python", "-m", "worker.consumer"]
