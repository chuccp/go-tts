#!/usr/bin/env python3
"""Create an INT8 Kokoro ONNX model for CPU inference."""

import argparse
from pathlib import Path

from onnxruntime.quantization import QuantType, quantize_dynamic


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path, help="FP32 Kokoro ONNX model")
    parser.add_argument("output", type=Path, help="Output INT8 ONNX model")
    args = parser.parse_args()

    args.output.parent.mkdir(parents=True, exist_ok=True)
    quantize_dynamic(
        str(args.input),
        str(args.output),
        weight_type=QuantType.QInt8,
        extra_options={"MatMulConstBOnly": True},
    )
    print(f"Wrote {args.output} ({args.output.stat().st_size / 1024 / 1024:.1f} MB)")


if __name__ == "__main__":
    main()
