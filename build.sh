#!/usr/bin/env bash
# 用 "." 构建整个包,避免逐个列文件名(stats.go 已删除,旧列表会构建失败)
go build -o ./app .
