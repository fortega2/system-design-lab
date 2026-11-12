# Sourced by the image entrypoint (keep it non-executable): applies every file
# in app/schema, then the monitoring role.
docker_process_init_files /lab/schema/* /lab/monitoring/*.sql
