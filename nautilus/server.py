import argparse
import signal
import sys
from concurrent import futures

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

from .config import load_config


def serve() -> int:
    parser = argparse.ArgumentParser(description="TradeFox Nautilus gRPC server")
    parser.add_argument("--port", type=int, default=0, help="gRPC port (default: from config)")
    args = parser.parse_args()

    config = load_config()
    port = args.port if args.port else config.grpc_port

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    # Register all servicers
    from .bridge_servicer import (
        StrategyServicer,
        ExecutionServicer,
        PortfolioServicer,
        BacktestServicer,
        DataServicer,
    )
    from .proto import nautilus_pb2_grpc

    nautilus_pb2_grpc.add_StrategyServiceServicer_to_server(StrategyServicer(), server)
    nautilus_pb2_grpc.add_ExecutionServiceServicer_to_server(ExecutionServicer(), server)
    nautilus_pb2_grpc.add_PortfolioServiceServicer_to_server(PortfolioServicer(), server)
    nautilus_pb2_grpc.add_BacktestServiceServicer_to_server(BacktestServicer(), server)
    nautilus_pb2_grpc.add_DataServiceServicer_to_server(DataServicer(), server)

    # Health check
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)

    server.add_insecure_port(f"[::]:{port}")
    server.start()
    print(f"Nautilus gRPC server listening on :{port}", flush=True)

    # Graceful shutdown
    stop_event = __import__("threading").Event()

    def _shutdown(signum, frame):
        print("Shutting down Nautilus gRPC server...", flush=True)
        stop_event.set()
        server.stop(grace=5)

    signal.signal(signal.SIGTERM, _shutdown)
    signal.signal(signal.SIGINT, _shutdown)

    server.wait_for_termination()
    return 0
