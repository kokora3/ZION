# Resource-Efficiency Baseline

ZION v0.1 is intended to remain practical on inexpensive commodity hardware. The following are engineering and benchmarking targets from `SPEC.md`, not consensus rules or guaranteed protocol limits:

- Idle CPU: low single-digit percentage where practical.
- Idle RAM: target below 512 MiB.
- NORMAL node: target below 1 GiB RAM under ordinary alpha load.
- VALIDATOR: target below 1 GiB RAM under ordinary alpha load.
- Queues and caches: bounded.
- Disk: bounded by configuration.

Targets may be revised after measurement without changing protocol compatibility. Resource efficiency is a security concern: future implementations must reject, drop, or defer work before peers can force unbounded CPU, memory, disk, connection, or queue growth.
