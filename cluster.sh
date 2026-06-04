#!/bin/bash

go build -o espbrew ./cmd/espbrew && ./espbrew cluster --role leader --port 8080
