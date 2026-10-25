#!/bin/bash

export DATABASE_PATH="./temperature.db"
export PORT="5500"
export FLUSH_INTERVAL_SEC="60"

go run ./cmd/tempmonitor
