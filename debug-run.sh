#!/bin/bash

export DATABASE_PATH="./temperature.db"
export PORT="5500"

go run ./cmd/api/main.go
