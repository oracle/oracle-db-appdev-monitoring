#!/bin/sh
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

: ${WEB_CONFIG:=/etc/prometheus/prometheus_vol/web.yml}
: ${PROM_CONFIG:=/etc/prometheus/prometheus_vol/config.yml}
: ${SECRETS_DIR:=/run/secrets}

# auth for prometheus -- web.yml
prom_pwd_sec_file=$SECRETS_DIR"/prom.auth.pwd"
sed -e "s|prom.auth.pwd|$(cat $prom_pwd_sec_file)|g" $WEB_CONFIG > /web.yml

# auth for exporter -- config.yml
exporter_usr_sec_file=$SECRETS_DIR"/auth.username"
exporter_pwd_sec_file=$SECRETS_DIR"/auth.password"
sed -e "s|auth.username|$(cat $exporter_usr_sec_file)|g" $PROM_CONFIG > /config_temp.yml
sed -e "s|auth.password|$(cat $exporter_pwd_sec_file)|g" /config_temp.yml > /config.yml

/bin/prometheus --config.file=/config.yml --web.config.file=/web.yml
