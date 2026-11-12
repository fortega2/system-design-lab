-- Read-only role for postgres_exporter.
CREATE USER exporter WITH PASSWORD 'exporter'; -- local only: the port is not published
GRANT pg_monitor TO exporter;
GRANT CONNECT ON DATABASE lab TO exporter;
