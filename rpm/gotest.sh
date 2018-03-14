#!/bin/bash
cd "$(dirname "$0")"
cd ../server/service
go test -v
