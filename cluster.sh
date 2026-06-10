#!/bin/bash

# Build everything (WASM UI + server) and start cluster
make build && ./espbrew cluster --role leader --port 8080
