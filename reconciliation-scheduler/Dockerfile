FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

COPY pyproject.toml ./
COPY scheduler ./scheduler

RUN pip install --no-cache-dir .

EXPOSE 8083

CMD ["python", "-m", "scheduler"]
