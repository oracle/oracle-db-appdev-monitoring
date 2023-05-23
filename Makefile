#
# Makefile Version 1.0
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#


deploy:
	docker stack deploy --compose-file docker-compose.yml oracledb-monitor

down:
	docker stack rm oracledb-monitor

log-oracledb:
	docker service logs --follow oracledb-monitor_oracledb --raw

log-exporter:
	docker service logs --follow oracledb-monitor_exporter --raw

# pause:
# 	docker-compose pause
#
# unpause:
# 	docker-compose unpause
