# Low Memory Tuning Guide

## Panel (Go binary)
The panel automatically optimizes for low-memory when no explicit GOMAXPROCS/GOGC/GOMEMLIMIT are set:
- GOMAXPROCS=1 (single thread)
- GOGC=50 (more frequent GC, lower peak memory)
- GOMEMLIMIT=100MB (soft memory cap)

Override with env vars if needed:
- `GOMAXPROCS=2` for multi-core
- `GOGC=100` for default GC behavior
- `GOMEMLIMIT=200000000` for 200MB limit

## MariaDB (recommended for 1GB RAM)
Add to `/etc/mysql/mariadb.conf.d/99-lowmem.cnf`:
```ini
[mysqld]
innodb_buffer_pool_size = 128M
innodb_log_buffer_size = 4M
key_buffer_size = 16M
max_connections = 30
thread_cache_size = 4
table_open_cache = 128
query_cache_size = 0
query_cache_type = 0
tmp_table_size = 16M
max_heap_table_size = 16M
```

## Nginx (recommended)
```nginx
worker_processes 1;
worker_connections 512;
```

## Node Agent
Already lightweight (~5MB RSS). No tuning needed.

## Expected Memory Usage (1GB server)
- MariaDB: ~200MB
- Nginx: ~20MB
- Panel binary: ~30-50MB
- FreeRADIUS: ~30MB
- OS + buffers: ~200MB
- Available: ~500MB headroom
