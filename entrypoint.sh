#!/bin/sh

# /usr/sbin/remoted &

prometheus --config.file=/etc/prometheus/prometheus.yml \
            --storage.tsdb.path=/prometheus \
            --web.console.libraries=/etc/prometheus/console_libraries \
            --web.console.templates=/etc/prometheus/consoles \
            --storage.tsdb.retention=200h \
            --web.enable-lifecycle \
            --web.enable-admin-api