#!/bin/bash

CGO_ENABLED=0 godep go build -a -tags netgo
