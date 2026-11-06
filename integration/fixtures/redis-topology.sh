#!/bin/sh
set -eu
if [ "$1" = cluster ]; then
  redis-server --port 17001 --cluster-enabled yes --cluster-config-file /tmp/cluster-17001.conf --cluster-node-timeout 1000 --cluster-announce-ip 127.0.0.1 --appendonly no --save '' &
  redis_pid_one=$!
  redis-server --port 17002 --cluster-enabled yes --cluster-config-file /tmp/cluster-17002.conf --cluster-node-timeout 1000 --cluster-announce-ip 127.0.0.1 --appendonly no --save '' &
  redis_pid_two=$!
  redis-server --port 17003 --cluster-enabled yes --cluster-config-file /tmp/cluster-17003.conf --cluster-node-timeout 1000 --cluster-announce-ip 127.0.0.1 --appendonly no --save '' &
  redis_pid_three=$!
  trap 'kill "$redis_pid_one" "$redis_pid_two" "$redis_pid_three"; wait' TERM INT EXIT
  until redis-cli -p 17001 ping && redis-cli -p 17002 ping && redis-cli -p 17003 ping; do sleep 0.2; done
  redis-cli --cluster create 127.0.0.1:17001 127.0.0.1:17002 127.0.0.1:17003 --cluster-replicas 0 --cluster-yes
  wait
else
  redis-server --port 17101 --appendonly no --save '' &
  redis_pid_one=$!
  redis-server --port 17102 --replicaof 127.0.0.1 17101 --appendonly no --save '' &
  redis_pid_two=$!
  cat > /tmp/sentinel.conf <<'CONFIG'
port 17103
bind 0.0.0.0
protected-mode no
sentinel monitor sdk-master 127.0.0.1 17101 1
sentinel down-after-milliseconds sdk-master 500
sentinel failover-timeout sdk-master 5000
sentinel parallel-syncs sdk-master 1
CONFIG
  redis-server /tmp/sentinel.conf --sentinel &
  redis_pid_three=$!
  trap 'kill "$redis_pid_one" "$redis_pid_two" "$redis_pid_three"; wait' TERM INT EXIT
  wait
fi
