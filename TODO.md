# TODO

## Features

- `--in-vm` / QEMU execution: running pipelines inside a QEMU VM (Python: `qemu.py`, `vm.py`)
- `--break [STAGE]`: debug shell that drops into bash during stage execution
- `OSBUILD_EXPORT_FORCE_NO_PRESERVE_OWNER`: environment variable check during export
- `OSBUILD_EXPERIMENTAL`: experimental flags support
- Solver infrastructure (`solver/dnf.py`, `solver/dnf5.py`): dependency resolution backends
