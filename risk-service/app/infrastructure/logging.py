"""Logs en JSON de una linea, siempre con correlation_id.

Sin esto, seguir un pago cruzando Go y Python es imposible: el
correlation_id que nace en core-api es el unico hilo comun.
"""

from __future__ import annotations

import logging
import sys
from typing import cast

import structlog


def configure(level: str = "INFO", service: str = "risk-service") -> None:
    logging.basicConfig(format="%(message)s", stream=sys.stdout, level=level.upper())
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso", utc=True),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.make_filtering_bound_logger(
            logging.getLevelNamesMapping()[level.upper()]
        ),
        cache_logger_on_first_use=True,
    )
    structlog.contextvars.bind_contextvars(service=service)


def logger(name: str = "risk-service") -> structlog.stdlib.BoundLogger:
    return cast(structlog.stdlib.BoundLogger, structlog.get_logger(name))
